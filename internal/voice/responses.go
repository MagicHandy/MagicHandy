package voice

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"
)

// applyResponse folds one worker frame into the request record.
func (s *Supervisor) applyResponse(pending *PendingRequest, response Response) (terminal bool, cancelWorker bool) {
	if pending.isCanceled() {
		switch response.Type {
		case ResponseTranscript, ResponseDone, ResponseCanceled, ResponseError:
			return true, false
		default:
			return false, false
		}
	}
	switch response.Type {
	case ResponseAudioChunk:
		if err := pending.appendAudio(response); err != nil {
			workerConn := pending.failAndCancel(&WorkerError{Code: ErrorCodeInternal, Message: "invalid worker audio: " + err.Error()})
			return true, workerConn != nil
		}
		return false, false
	case ResponseTranscript:
		if err := pending.completeTranscript(response); err != nil {
			workerConn := pending.failAndCancel(&WorkerError{Code: ErrorCodeInternal, Message: "invalid worker transcript: " + err.Error()})
			return true, workerConn != nil
		}
		return true, false
	case ResponseDone:
		if err := pending.completeAudio(); err != nil {
			workerConn := pending.failAndCancel(&WorkerError{Code: ErrorCodeInternal, Message: "invalid worker audio: " + err.Error()})
			return true, workerConn != nil
		}
		return true, false
	case ResponseCanceled:
		pending.markCanceled()
		return true, false
	case ResponseError:
		workerErr := response.Error
		if workerErr == nil {
			workerErr = &WorkerError{Code: ErrorCodeInternal, Message: "worker reported an error"}
		}
		pending.fail(workerErr)
		return true, false
	default:
		workerConn := pending.failAndCancel(&WorkerError{
			Code: ErrorCodeInternal, Message: fmt.Sprintf("invalid worker response type %q", response.Type),
		})
		return true, workerConn != nil
	}
}

func (p *PendingRequest) appendAudio(response Response) error {
	if p.Type != RequestSpeak {
		return errors.New("audio was returned for a non-speak request")
	}
	format := strings.ToLower(strings.TrimSpace(response.AudioFormat))
	if format == "" {
		return errors.New("audio format is missing")
	}
	data, err := base64.StdEncoding.DecodeString(response.AudioB64)
	if err != nil || len(data) == 0 {
		return errors.New("audio payload is empty or invalid base64")
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if response.Seq != p.chunks {
		return fmt.Errorf("audio sequence %d arrived; expected %d", response.Seq, p.chunks)
	}
	if p.audioFormat != "" && p.audioFormat != format {
		return fmt.Errorf("audio format changed from %q to %q", p.audioFormat, format)
	}
	if len(p.audio)+len(data) > maxRetainedAudioBytes {
		p.audioTruncated = true
		return fmt.Errorf("audio exceeds the %d MiB retention limit", maxRetainedAudioBytes>>20)
	}
	if p.audioFormat == "" {
		p.audioFormat = format
	}
	p.audio = append(p.audio, data...)
	p.chunks++
	return nil
}

func (p *PendingRequest) completeAudio() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Type != RequestSpeak {
		return errors.New("done was returned for a non-speak request")
	}
	if p.chunks == 0 || len(p.audio) == 0 {
		return errors.New("request completed without audio")
	}
	if p.audioTruncated {
		return errors.New("request completed with truncated audio")
	}
	if p.audioFormat == "pcm_s16le_24000" && len(p.audio)%2 != 0 {
		return errors.New("PCM audio ended on an incomplete sample")
	}
	p.state = RequestStateDone
	return nil
}

func (p *PendingRequest) completeTranscript(response Response) error {
	if p.Type != RequestTranscribe {
		return errors.New("transcript was returned for a non-transcription request")
	}
	if response.Rejected != "" {
		if response.Rejected != RejectedNoSpeech && response.Rejected != RejectedLowConfidence {
			return fmt.Errorf("unknown rejection reason %q", response.Rejected)
		}
		if len(response.Candidates) != 0 {
			return errors.New("rejected transcript also contained candidates")
		}
	} else if len(response.Candidates) == 0 {
		return errors.New("transcript contained no candidates or rejection reason")
	}
	for _, candidate := range response.Candidates {
		if strings.TrimSpace(candidate.Text) == "" {
			return errors.New("transcript candidate text is empty")
		}
		if math.IsNaN(candidate.Confidence) || math.IsInf(candidate.Confidence, 0) || candidate.Confidence < 0 || candidate.Confidence > 1 {
			return fmt.Errorf("transcript confidence %v is outside 0..1", candidate.Confidence)
		}
	}

	p.mu.Lock()
	p.transcript = append([]TranscriptCandidate(nil), response.Candidates...)
	p.rejected = response.Rejected
	p.state = RequestStateDone
	p.mu.Unlock()
	return nil
}
