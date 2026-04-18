package dsp

import (
	"errors"
	"log"
	"math"
	"math/cmplx"
	"os"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"gonum.org/v1/gonum/dsp/fourier"
)

type MatchEQEngine struct {
	TargetSR  int
	NFFT      int
	HopLength int
	IRLen     int
	LastIR    []float64
}

func NewMatchEQEngine(targetSR int) *MatchEQEngine {
	return &MatchEQEngine{
		TargetSR:  targetSR,
		NFFT:      8192,
		HopLength: 2048,
		IRLen:     2048,
	}
}

func readWavMono(filePath string) ([]float64, int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	decoder := wav.NewDecoder(f)
	buf, err := decoder.FullPCMBuffer()
	if err != nil {
		return nil, 0, err
	}

	sr := int(buf.Format.SampleRate)
	channels := buf.Format.NumChannels
	var mono []float64

	norm := math.Pow(2, float64(buf.SourceBitDepth-1))
	if norm <= 0 { norm = 1.0 }

	for i := 0; i < len(buf.Data); i += channels {
		sum := 0.0
		for c := 0; c < channels; c++ {
			sum += float64(buf.Data[i+c])
		}
		mono = append(mono, sum/float64(channels)/norm)
	}
	return mono, sr, nil
}

func (e *MatchEQEngine) correlateDelay(ref, tgt []float64) int {
	limit := e.TargetSR / 2
	if len(ref) > limit { ref = ref[:limit] }
	if len(tgt) > limit { tgt = tgt[:limit] }

	bestOffset := 0
	maxCorr := -1.0
	maxShift := 2000
	if len(tgt) < maxShift { maxShift = len(tgt) - 1 }

	for off := -maxShift; off <= maxShift; off++ {
		var sum float64
		startR := 0
		startT := -off
		if off > 0 { startR = off; startT = 0 }
		length := len(ref) - startR
		if len(tgt)-startT < length { length = len(tgt) - startT }
		if length <= 0 { continue }
		for i := 0; i < length; i++ { sum += ref[startR+i] * tgt[startT+i] }
		if sum > maxCorr { maxCorr = sum; bestOffset = off }
	}
	return bestOffset
}

func (e *MatchEQEngine) padFront(slice []float64, n int) []float64 {
	padded := make([]float64, len(slice)+n)
	copy(padded[n:], slice)
	return padded
}

func (e *MatchEQEngine) analyzeSpectrum(audio []float64) []float64 {
	cfft := fourier.NewCmplxFFT(e.NFFT)
	numFrames := (len(audio) - e.NFFT) / e.HopLength
	if numFrames <= 0 { return make([]float64, e.NFFT/2+1) }

	sumMag := make([]float64, e.NFFT/2+1)
	hanning := make([]float64, e.NFFT)
	for i := 0; i < e.NFFT; i++ {
		hanning[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(e.NFFT-1)))
	}

	frameEnergies := make([]float64, numFrames)
	for i := 0; i < numFrames; i++ {
		start := i * e.HopLength
		frame := audio[start : start+e.NFFT]
		for _, v := range frame { frameEnergies[i] += v * v }
	}

	var maxE float64
	for _, en := range frameEnergies { if en > maxE { maxE = en } }
	threshold := maxE * 0.1

	validCount := 0
	buffer := make([]complex128, e.NFFT)
	coeffs := make([]complex128, e.NFFT)

	for i := 0; i < numFrames; i++ {
		if frameEnergies[i] < threshold { continue }
		start := i * e.HopLength
		for j := 0; j < e.NFFT; j++ {
			buffer[j] = complex(audio[start+j]*hanning[j], 0)
		}
		// In Gonum, Coefficients(dst, src)
		cfft.Coefficients(coeffs, buffer)
		for j := 0; j <= e.NFFT/2; j++ {
			sumMag[j] += cmplx.Abs(coeffs[j])
		}
		validCount++
	}

	for j := range sumMag {
		sumMag[j] /= math.Max(1.0, float64(validCount))
	}

	smoothed := make([]float64, len(sumMag))
	window := 7
	for i := range sumMag {
		sum := 0.0
		count := 0.0
		for k := -window/2; k <= window/2; k++ {
			idx := i + k
			if idx >= 0 && idx < len(sumMag) {
				sum += sumMag[idx]
				count++
			}
		}
		smoothed[i] = sum / count
	}
	return smoothed
}

func (e *MatchEQEngine) synthesizeIR(hF []float64) []float64 {
	targetLen := e.IRLen/2 + 1
	hInterp := make([]float64, targetLen)
	
	for i := 0; i < targetLen; i++ {
		origIdx := float64(i) * float64(len(hF)-1) / float64(targetLen-1)
		origLower := int(math.Floor(origIdx))
		origUpper := origLower + 1
		if origUpper >= len(hF) { origUpper = len(hF) - 1 }
		frac := origIdx - float64(origLower)
		val := hF[origLower]*(1-frac) + hF[origUpper]*frac
		hInterp[i] = math.Max(val, 1e-10) 
	}

	logMag := make([]float64, targetLen)
	for i, v := range hInterp { logMag[i] = math.Log(v) }

	fullLogMag := make([]complex128, e.IRLen)
	for i := 0; i < targetLen; i++ {
		fullLogMag[i] = complex(logMag[i], 0)
	}
	for i := 1; i < targetLen-1; i++ {
		fullLogMag[e.IRLen-i] = fullLogMag[i]
	}

	cfft := fourier.NewCmplxFFT(e.IRLen)
	cepstrumC := make([]complex128, e.IRLen)
	// Sequence(dst, src)
	cfft.Sequence(cepstrumC, fullLogMag) // IFFT
	
	scale := 1.0 / float64(e.IRLen)
	cepstrum := make([]float64, e.IRLen)
	for i, v := range cepstrumC { cepstrum[i] = real(v) * scale }

	mpWindow := make([]float64, e.IRLen)
	mpWindow[0] = 1
	for i := 1; i < e.IRLen/2; i++ { mpWindow[i] = 2 }
	mpWindow[e.IRLen/2] = 1

	for i := 0; i < e.IRLen; i++ {
		cepstrumC[i] = complex(cepstrum[i] * mpWindow[i], 0)
	}

	minPhaseSpec := make([]complex128, e.IRLen)
	cfft.Coefficients(minPhaseSpec, cepstrumC) // FFT

	for i, v := range minPhaseSpec { minPhaseSpec[i] = cmplx.Exp(v) }

	irC := make([]complex128, e.IRLen)
	cfft.Sequence(irC, minPhaseSpec) // IFFT

	ir := make([]float64, e.IRLen)
	for i, v := range irC { ir[i] = real(v) * scale }

	// Phase Smear
	hIrCoeffs := make([]complex128, e.IRLen)
	for i := 0; i < e.IRLen; i++ { irC[i] = complex(ir[i], 0) }
	cfft.Coefficients(hIrCoeffs, irC)

	for i := 0; i <= e.IRLen/2; i++ {
		w := math.Pi * float64(i) / float64(e.IRLen/2)
		delay := 150.0 * (1.0 - float64(i)/float64(e.IRLen/2))
		shift := cmplx.Exp(complex(0, -delay*w))
		
		hIrCoeffs[i] *= shift
		if i > 0 && i < e.IRLen/2 {
			hIrCoeffs[e.IRLen-i] *= cmplx.Conj(shift)
		}
	}
	
	cfft.Sequence(irC, hIrCoeffs)
	for i := range ir { ir[i] = real(irC[i]) * scale }

	maxAbs := 0.0
	maxIdx := 0
	for i, v := range ir {
		if math.Abs(v) > maxAbs { maxAbs = math.Abs(v); maxIdx = i }
	}
	if ir[maxIdx] < 0 {
		for i := range ir { ir[i] = -ir[i] }
	}

	fadeLen := int(float64(e.IRLen) * 0.08)
	for i := 0; i < fadeLen; i++ {
		frac := 1.0 - float64(i)/float64(fadeLen-1)
		ir[e.IRLen-fadeLen+i] *= (frac * frac)
	}

	return ir
}

type AnalysisResult struct {
	RefSpectrum []float64 `json:"ref_spectrum"`
	TgtSpectrum []float64 `json:"tgt_spectrum"`
	IRSpectrum  []float64 `json:"ir_spectrum"`
}

func (e *MatchEQEngine) Analyze(refPath, tgtPath string, progressCallback func(string, int)) (*AnalysisResult, error) {
	if progressCallback != nil { progressCallback("Cargando audios...", 5) }
	
	refAudio, srF, err := readWavMono(refPath)
	if err != nil { return nil, err }
	if srF != e.TargetSR { return nil, errors.New("Reference audio is not target SR") }
	tgtAudio, srT, err := readWavMono(tgtPath)
	if err != nil { return nil, err }
	if srT != e.TargetSR { return nil, errors.New("Target audio is not target SR") }

	if progressCallback != nil { progressCallback("Alineando audios...", 25) }
	offset := e.correlateDelay(refAudio, tgtAudio)
	log.Printf("Offset: %d", offset)

	if offset > 0 {
		tgtAudio = e.padFront(tgtAudio, offset)
	} else if offset < 0 {
		if -offset < len(tgtAudio) {
			tgtAudio = tgtAudio[-offset:]
		}
	}
	
	length := len(refAudio)
	if len(tgtAudio) < length { length = len(tgtAudio) }
	refAudio = refAudio[:length]
	tgtAudio = tgtAudio[:length]

	if progressCallback != nil { progressCallback("Analizando espectro A...", 35) }
	magRef := e.analyzeSpectrum(refAudio)
	
	if progressCallback != nil { progressCallback("Analizando espectro B...", 55) }
	magTgt := e.analyzeSpectrum(tgtAudio)

	if progressCallback != nil { progressCallback("Calculando respuesta...", 70) }
	
	hF := make([]float64, len(magRef))
	magIR := make([]float64, len(magRef))
	
	for i := 0; i < len(hF); i++ {
		r := magRef[i]
		t := math.Max(magTgt[i], 1e-10)
		diffRatio := math.Max(0.01, math.Min(100.0, r/t))
		
		freq := float64(i) * float64(e.TargetSR) / float64(e.NFFT)
		fade := 1.0
		if freq < 80 {
			fade = math.Max(0, (freq-40)/40.0)
		} else if freq > 10000 {
			fade = math.Max(0, 1.0-(freq-10000)/6000.0)
		}
		
		hF[i] = 1.0 + (diffRatio - 1.0)*fade
		magIR[i] = 20 * math.Log10(math.Max(hF[i], 1e-10))
		magRef[i] = 20 * math.Log10(math.Max(r, 1e-10))
		magTgt[i] = 20 * math.Log10(math.Max(t, 1e-10))
	}

	if progressCallback != nil { progressCallback("Sintetizando IR...", 80) }
	e.LastIR = e.synthesizeIR(hF)

	if progressCallback != nil { progressCallback("Listo!", 100) }

	return &AnalysisResult{
		RefSpectrum: reduceArr(magRef, 200),
		TgtSpectrum: reduceArr(magTgt, 200),
		IRSpectrum:  reduceArr(magIR, 200),
	}, nil
}

func reduceArr(arr []float64, maxPts int) []float64 {
	if len(arr) <= maxPts { return arr }
	res := make([]float64, maxPts)
	step := float64(len(arr)) / float64(maxPts)
	for i := 0; i < maxPts; i++ {
		idx := int(float64(i) * step)
		res[i] = arr[idx]
	}
	return res
}

func (e *MatchEQEngine) ExportIR(outputPath string, progressCallback func(string, int)) error {
	if e.LastIR == nil { return errors.New("No hay IR generado") }

	if progressCallback != nil { progressCallback("Normalizando a nivel estandar...", 60) }
	
	peak := 0.0
	for _, v := range e.LastIR {
		if math.Abs(v) > peak { peak = math.Abs(v) }
	}

	normIR := make([]int, len(e.LastIR))
	if peak > 0 {
		for i, v := range e.LastIR {
			val := (v / peak * 0.99) * 8388607.0
			normIR[i] = int(val)
		}
	}

	if progressCallback != nil { progressCallback("Escribiendo WAV...", 85) }

	f, err := os.Create(outputPath)
	if err != nil { return err }
	defer f.Close()

	enc := wav.NewEncoder(f, e.TargetSR, 24, 1, 1)
	buf := &audio.IntBuffer{Data: normIR, Format: &audio.Format{NumChannels: 1, SampleRate: e.TargetSR}}
	
	if err := enc.Write(buf); err != nil { return err }
	if err := enc.Close(); err != nil { return err }

	if progressCallback != nil { progressCallback("Guardado", 100) }
	return nil
}
