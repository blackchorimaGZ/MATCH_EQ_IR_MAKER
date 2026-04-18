import './style.css';
import Chart from 'chart.js/auto';
// @ts-ignore
import { SelectAudioFile, RunAnalysis, ExportIRFile } from '../wailsjs/go/main/App';
// @ts-ignore
import { EventsOn } from '../wailsjs/runtime/runtime';

let refPath = "";
let tgtPath = "";
let chart: any = null;

const initChart = () => {
    const ctx = document.getElementById('spectrumChart') as HTMLCanvasElement;
    chart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: Array.from({length: 200}, (_, i) => Math.round(i * (24000/200))),
            datasets: [
                {
                    label: 'Referencia (A)',
                    data: [],
                    borderColor: 'rgba(0, 255, 255, 1)',
                    borderWidth: 1.5,
                    pointRadius: 0,
                    tension: 0.1
                },
                {
                    label: 'Objetivo (B)',
                    data: [],
                    borderColor: 'rgba(128, 128, 128, 1)',
                    borderWidth: 1.5,
                    pointRadius: 0,
                    tension: 0.1
                },
                {
                    label: 'IR Generado',
                    data: [],
                    borderColor: 'rgba(170, 0, 255, 1)',
                    borderWidth: 2,
                    pointRadius: 0,
                    tension: 0.1
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                x: {
                    type: 'linear',
                    min: 50,
                    max: 12000,
                    grid: { color: '#333' }
                },
                y: {
                    min: -40,
                    max: 40,
                    grid: { color: '#333' }
                }
            },
            plugins: {
                legend: { labels: { color: '#ccc' } }
            }
        }
    });
};

document.addEventListener("DOMContentLoaded", () => {
    initChart();

    const lblRef = document.getElementById('lblRef');
    const lblTgt = document.getElementById('lblTgt');
    const statusText = document.getElementById('statusText');
    const progressFill = document.getElementById('progressFill');
    const btnExport = document.getElementById('btnExport') as HTMLButtonElement;

    EventsOn("progress", (data: any) => {
        if(statusText) statusText.innerText = data.status;
        if(progressFill) progressFill.style.width = `${data.val}%`;
    });

    document.getElementById('btnRef')?.addEventListener('click', async () => {
        const path = await SelectAudioFile();
        if (path) {
            refPath = path;
            if(lblRef) lblRef.innerText = path.split('\\').pop() || path;
        }
    });

    document.getElementById('btnTgt')?.addEventListener('click', async () => {
        const path = await SelectAudioFile();
        if (path) {
            tgtPath = path;
            if(lblTgt) lblTgt.innerText = path.split('\\').pop() || path;
        }
    });

    document.getElementById('btnAnalyze')?.addEventListener('click', async () => {
        if (!refPath || !tgtPath) {
            alert("Faltan archivos por cargar.");
            return;
        }
        btnExport.disabled = true;
        const result = await RunAnalysis(refPath, tgtPath);
        if (result) {
            chart.data.datasets[0].data = result.ref_spectrum;
            chart.data.datasets[1].data = result.tgt_spectrum;
            chart.data.datasets[2].data = result.ir_spectrum;
            chart.update();
            btnExport.disabled = false;
        }
    });

    btnExport?.addEventListener('click', async () => {
        await ExportIRFile();
    });
});
