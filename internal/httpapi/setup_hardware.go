package httpapi

import (
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func setupHardwareSnapshot() map[string]any {
	snapshot := map[string]any{
		"platform": runtime.GOOS + "/" + runtime.GOARCH,
		"nvidia":   false,
		"cuda":     false,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits") // #nosec G204 -- fixed diagnostic command.
	configureSetupProcess(command)
	if output, err := command.Output(); err == nil {
		name, memory := largestNVIDIAGPU(string(output))
		if name != "" {
			snapshot["nvidia"] = true
			snapshot["gpu_name"] = name
		}
		if memory != "" {
			snapshot["vram_mib"] = memory
		}
	}
	if _, err := exec.LookPath("nvcc"); err == nil {
		snapshot["cuda"] = true
	}
	return snapshot
}

// largestNVIDIAGPU picks the card with the most memory from nvidia-smi's
// "name, MiB" lines, so a machine with two GPUs is judged by the one a model
// would load on rather than by an unparsable joined line.
func largestNVIDIAGPU(output string) (string, string) {
	bestName, bestMemory, bestMiB := "", "", -1
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		name, memory, found := strings.Cut(line, ",")
		name, memory = strings.TrimSpace(name), strings.TrimSpace(memory)
		if name == "" {
			continue
		}
		mib := 0
		if found {
			if value, err := strconv.Atoi(memory); err == nil && value > 0 {
				mib = value
			} else {
				memory = ""
			}
		}
		if mib > bestMiB {
			bestName, bestMemory, bestMiB = name, memory, mib
		}
	}
	return bestName, bestMemory
}
