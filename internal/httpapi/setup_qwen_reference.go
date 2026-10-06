package httpapi

import (
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type setupQwenReference struct {
	WAV        string `json:"wav"`
	Transcript string `json:"transcript"`
}

func normalizeSetupQwenReference(reference *setupQwenReference) (*setupQwenReference, error) {
	if reference == nil {
		return nil, nil
	}
	result := &setupQwenReference{WAV: strings.TrimSpace(reference.WAV), Transcript: strings.TrimSpace(reference.Transcript)}
	if result.Transcript == "" || len(result.Transcript) > 8192 {
		return nil, errors.New("enter the exact words spoken in the voice sample (up to 8192 bytes)")
	}
	if _, err := checkSetupQwenWAV(result.WAV); err != nil {
		return nil, err
	}
	return result, nil
}

// This checks a bounded local WAV's container and duration. It does not claim
// to recognize speech, match the transcript or measure voice-cloning quality.
func checkSetupQwenWAV(path string) (int, error) {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".wav") {
		return 0, errors.New("choose a WAV audio sample on the computer running MagicHandy")
	}
	file, err := os.Open(path) // #nosec G304 -- explicit host-authorized local WAV selection.
	if err != nil {
		return 0, errors.New("the voice sample could not be read; choose an existing WAV file")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	const limit = 16 << 20
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return 0, errors.New("choose a regular WAV file smaller than 16 MiB")
	}
	audio, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(audio) > limit {
		return 0, errors.New("the voice sample could not be read; choose an existing WAV file")
	}
	return setupQwenWAVDuration(audio)
}

func setupQwenWAVDuration(audio []byte) (int, error) {
	invalid := errors.New("the sample is not a supported WAV; use PCM or floating-point WAV audio with one or two channels")
	if len(audio) < 12 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || uint64(binary.LittleEndian.Uint32(audio[4:8]))+8 != uint64(len(audio)) {
		return 0, invalid
	}
	var rate, align, dataBytes int
	offset := 12
	for offset+8 <= len(audio) {
		size := int64(binary.LittleEndian.Uint32(audio[offset+4 : offset+8]))
		if size > int64(len(audio)-offset-8) {
			return 0, invalid
		}
		chunk := audio[offset+8 : offset+8+int(size)]
		switch string(audio[offset : offset+4]) {
		case "fmt ":
			if rate != 0 {
				return 0, invalid
			}
			var ok bool
			rate, align, ok = setupQwenWAVFormat(chunk)
			if !ok {
				return 0, invalid
			}
		case "data":
			if dataBytes != 0 {
				return 0, invalid
			}
			dataBytes = len(chunk)
		}
		offset += 8 + int(size) + int(size%2)
	}
	if offset != len(audio) || rate == 0 || dataBytes == 0 || dataBytes%align != 0 {
		return 0, invalid
	}
	duration := int64(dataBytes) * 1000 / int64(rate)
	if duration < 1000 || duration > 30000 {
		return 0, errors.New("use a voice sample between 1 and 30 seconds; 3 to 10 seconds is recommended")
	}
	return int(duration), nil // #nosec G115 -- validated in [1000,30000] above.
}

func setupQwenWAVFormat(chunk []byte) (int, int, bool) {
	if len(chunk) < 16 {
		return 0, 0, false
	}
	format, channels := binary.LittleEndian.Uint16(chunk), binary.LittleEndian.Uint16(chunk[2:4])
	sampleRate, bits := binary.LittleEndian.Uint32(chunk[4:8]), binary.LittleEndian.Uint16(chunk[14:16])
	validBits := format == 1 && slices.Contains([]uint16{8, 16, 24, 32}, bits) || format == 3 && slices.Contains([]uint16{32, 64}, bits)
	if channels < 1 || channels > 2 || sampleRate < 8000 || sampleRate > 96000 || !validBits {
		return 0, 0, false
	}
	align := channels * bits / 8
	rate := sampleRate * uint32(align)
	if binary.LittleEndian.Uint16(chunk[12:14]) != align || binary.LittleEndian.Uint32(chunk[8:12]) != rate {
		return 0, 0, false
	}
	return int(rate), int(align), true
}

func (s *Server) handleSetupQwenReferenceCheck(w http.ResponseWriter, r *http.Request) {
	if !s.requireController(w, r) {
		return
	}
	if !s.capabilities(r).ConfigureHost {
		writeError(w, http.StatusForbidden, errors.New(administratorHostAccessRequired))
		return
	}
	var request struct {
		WAV string `json:"wav"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	duration, err := checkSetupQwenWAV(strings.TrimSpace(request.WAV))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"duration_ms": duration})
}
