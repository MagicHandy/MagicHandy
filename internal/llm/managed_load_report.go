package llm

import (
	"bytes"
	"regexp"
	"strconv"
	"sync"
)

// ManagedLoadReport is what the managed runner said about placing the model
// while it loaded. A model that does not fully fit on the GPU still runs, but
// every reply slows down, which users otherwise only notice as lag.
type ManagedLoadReport struct {
	// OffloadedLayers and TotalLayers come from llama.cpp's
	// "offloaded N/M layers to GPU" line; both are zero until it appears.
	OffloadedLayers int `json:"offloaded_layers"`
	TotalLayers     int `json:"total_layers"`
}

// FullyOffloaded reports a load that put every layer on the GPU.
func (report ManagedLoadReport) FullyOffloaded() bool {
	return report.TotalLayers > 0 && report.OffloadedLayers >= report.TotalLayers
}

var offloadedLayersPattern = regexp.MustCompile(`offloaded (\d+)/(\d+) layers to GPU`)

const loadReportLineLimit = 4096

// loadReportWatcher scans runner output line by line for load facts. The
// runner's own tail buffer keeps only the latest output, which no longer
// holds the load lines once the server is serving.
type loadReportWatcher struct {
	mu      sync.Mutex
	partial []byte
	report  ManagedLoadReport
}

func (w *loadReportWatcher) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.partial = append(w.partial, data...)
	for {
		index := bytes.IndexByte(w.partial, '\n')
		if index < 0 {
			break
		}
		w.scanLine(w.partial[:index])
		w.partial = w.partial[index+1:]
	}
	if len(w.partial) > loadReportLineLimit {
		w.partial = append([]byte(nil), w.partial[len(w.partial)-loadReportLineLimit:]...)
	}
	return len(data), nil
}

func (w *loadReportWatcher) scanLine(line []byte) {
	if !bytes.Contains(line, []byte("offloaded")) {
		return
	}
	match := offloadedLayersPattern.FindSubmatch(line)
	if match == nil {
		return
	}
	offloaded, offloadedErr := strconv.Atoi(string(match[1]))
	total, totalErr := strconv.Atoi(string(match[2]))
	if offloadedErr == nil && totalErr == nil {
		w.report = ManagedLoadReport{OffloadedLayers: offloaded, TotalLayers: total}
	}
}

func (w *loadReportWatcher) Reset() {
	w.mu.Lock()
	w.partial = nil
	w.report = ManagedLoadReport{}
	w.mu.Unlock()
}

func (w *loadReportWatcher) Report() ManagedLoadReport {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.report
}
