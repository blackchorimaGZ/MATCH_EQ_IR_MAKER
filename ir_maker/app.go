package main

import (
	"context"
	"fmt"
	"ir_maker/dsp"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx    context.Context
	engine *dsp.MatchEQEngine
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		engine: dsp.NewMatchEQEngine(48000),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) SelectAudioFile() string {
	filepath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Selecciona un archivo de audio WAV",
		Filters: []runtime.FileFilter{
			{DisplayName: "Archivos WAV", Pattern: "*.wav"},
		},
	})
	if err != nil {
		return ""
	}
	return filepath
}

func (a *App) RunAnalysis(refPath, tgtPath string) *dsp.AnalysisResult {
	if refPath == "" || tgtPath == "" {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Error",
			Message: "Faltan archivos de audio.",
		})
		return nil
	}

	progressCb := func(status string, percent int) {
		runtime.EventsEmit(a.ctx, "progress", map[string]interface{}{
			"status": status,
			"val":    percent,
		})
	}

	res, err := a.engine.Analyze(refPath, tgtPath, progressCb)
	if err != nil {
		runtime.EventsEmit(a.ctx, "progress", map[string]interface{}{
			"status": fmt.Sprintf("Error: %v", err),
			"val":    0,
		})
		return nil
	}
	return res
}

func (a *App) ExportIRFile() {
	if a.engine.LastIR == nil {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.WarningDialog,
			Title:   "Aviso",
			Message: "Aún no hay un IR generado. Ejecuta el análisis primero.",
		})
		return
	}

	filepath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Guardar IR WAV",
		DefaultFilename: "IR - Match_EQ.wav",
		Filters: []runtime.FileFilter{
			{DisplayName: "Archivos WAV", Pattern: "*.wav"},
		},
	})

	if err != nil || filepath == "" {
		return
	}

	progressCb := func(status string, percent int) {
		runtime.EventsEmit(a.ctx, "progress", map[string]interface{}{
			"status": status,
			"val":    percent,
		})
	}

	err = a.engine.ExportIR(filepath, progressCb)
	if err != nil {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Error de Guardado",
			Message: err.Error(),
		})
	} else {
		runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.InfoDialog,
			Title:   "Éxito",
			Message: "¡IR Guardado correctamente!",
		})
	}
}
