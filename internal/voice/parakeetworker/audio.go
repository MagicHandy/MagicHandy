package parakeetworker

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"strings"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

func openAudio(request protocol.Request) (io.ReadCloser, int64, error) {
	format := strings.ToLower(strings.TrimSpace(request.AudioFormat))
	if format != "" && format != "wav" && format != "webm" && format != "ogg" {
		return nil, 0, fmt.Errorf("audio format must be wav, webm, or ogg")
	}
	if request.AudioB64 != "" {
		if len(request.AudioB64) > base64.StdEncoding.EncodedLen(maxAudioBytes) {
			return nil, 0, fmt.Errorf("audio_b64 exceeds %d MiB", maxAudioBytes>>20)
		}
		audio, err := base64.StdEncoding.DecodeString(request.AudioB64)
		if err != nil {
			return nil, 0, fmt.Errorf("audio_b64 is not valid base64")
		}
		if len(audio) > maxAudioBytes {
			return nil, 0, fmt.Errorf("audio_b64 exceeds %d MiB", maxAudioBytes>>20)
		}
		return io.NopCloser(bytes.NewReader(audio)), int64(len(audio)), nil
	}
	if request.AudioRef == "" {
		return io.NopCloser(strings.NewReader("")), 0, nil
	}
	// #nosec G304 -- audio_ref is a private core-owned staging file received over stdio.
	file, err := os.Open(request.AudioRef)
	if err != nil {
		return nil, 0, fmt.Errorf("audio_ref file is unavailable")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, 0, fmt.Errorf("audio_ref file is unavailable")
	}
	if info.Size() > maxAudioBytes {
		_ = file.Close()
		return nil, 0, fmt.Errorf("audio_ref file exceeds %d MiB", maxAudioBytes>>20)
	}
	return file, info.Size(), nil
}

func multipartAudio(audio io.Reader, size int64, format, model string) (io.Reader, string, int64, error) {
	var header bytes.Buffer
	form := multipart.NewWriter(&header)
	if model != "" {
		if err := form.WriteField("model", model); err != nil {
			return nil, "", 0, err
		}
	}
	if format == "" {
		format = "wav"
	}
	if _, err := form.CreateFormFile("file", "audio."+format); err != nil {
		return nil, "", 0, err
	}
	prefix := append([]byte(nil), header.Bytes()...)
	header.Reset()
	if err := form.Close(); err != nil {
		return nil, "", 0, err
	}
	tail := header.Bytes()
	body := io.MultiReader(bytes.NewReader(prefix), io.LimitReader(audio, size), bytes.NewReader(tail))
	return body, form.FormDataContentType(), int64(len(prefix)) + size + int64(len(tail)), nil
}
