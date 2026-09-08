package openaittsworker

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
)

func (s *session) streamAudio(ctx context.Context, requestID string, reader io.Reader, format string) {
	buffer := make([]byte, chunkBytes)
	seq, total := 0, 0
	for {
		n, err := reader.Read(buffer)
		if ctx.Err() != nil || s.isCanceled(requestID) {
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: requestID})
			return
		}
		if n > 0 {
			total += n
			if total > maxAudioBytes {
				s.sendError(requestID, protocol.ErrorCodeInternal, fmt.Sprintf("TTS audio exceeds %d MiB", maxAudioBytes>>20), false)
				return
			}
			s.send(protocol.Response{Type: protocol.ResponseAudioChunk, RequestID: requestID, Seq: seq, AudioFormat: format, AudioB64: base64.StdEncoding.EncodeToString(buffer[:n])})
			seq++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			s.sendError(requestID, protocol.ErrorCodeInternal, "TTS audio stream was interrupted: "+sanitize(err.Error(), s.options.APIKey), true)
			return
		}
	}
	if total == 0 {
		s.sendError(requestID, protocol.ErrorCodeInternal, "TTS server returned no audio", true)
		return
	}
	s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: requestID})
}
