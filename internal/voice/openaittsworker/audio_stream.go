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
			// An owned model can remain alive after a CUDA allocation/capture
			// failure but be unusable for every later request. Do not report it
			// as ready just because its Python process still exists. Intentional
			// cancellation above is excluded from this recovery path.
			if s.server != nil {
				s.setLoaded(false)
				s.setManagedFailure("Local speech generation was interrupted. Restart voice output and try again: " + sanitize(err.Error(), s.options.APIKey))
			}
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
