// Package elevenlabsworker implements the ADR 0003 worker protocol for the
// ElevenLabs cloud TTS API (ADR 0007's premium non-Python path). It is pure
// Go HTTP â€” no Python, no CGo â€” and runs as a separate worker process so a
// cloud outage or bad key can never take the core app down. The API key is
// a private credential: it arrives only via the ELEVENLABS_API_KEY
// environment variable, is sent only as the xi-api-key header, and never
// appears in frames, logs, or errors.
package elevenlabsworker

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mapledaemon/MagicHandy/internal/voice/elevenlabs"
	"github.com/mapledaemon/MagicHandy/internal/voice/protocol"
	"github.com/mapledaemon/MagicHandy/internal/voice/workerhost"
)

// Defaults for the ElevenLabs API. Voice and model are overridable worker
// arguments chosen in Settings â†’ Voice.
const (
	DefaultBaseURL = elevenlabs.DefaultBaseURL
	// DefaultVoiceID is "Jessica", a current default voice every plan can
	// use through the API. Keep it equal to config.DefaultElevenLabsVoiceID.
	DefaultVoiceID = "cgSgspJ2msm6clMCkdW9"
	// DefaultModelID matches config.DefaultElevenLabsModelID.
	DefaultModelID = "eleven_v4_turbo"
	// OutputFormatAuto streams raw PCM for progressive playback when the reply
	// fits the core's audio bound, and falls back to MP3 for long replies.
	OutputFormatAuto = "auto"
	// DefaultOutputFormat is kept for command-line compatibility.
	DefaultOutputFormat = OutputFormatAuto

	providerName    = "elevenlabs"
	providerVersion = "2.0.0"

	loadTimeout    = 20 * time.Second
	chunkBytes     = 32 * 1024
	queueCapacity  = 8
	maxSpeechBytes = 32 << 10
	maxAudioBytes  = protocol.MaxAudioBytes

	// pcmOutputFormat is raw 16-bit little-endian mono PCM at 24 kHz, which the
	// core and browser already play progressively as pcm_s16le_24000.
	pcmOutputFormat = "pcm_24000"
	pcmFormatLabel  = "pcm_s16le_24000"
	// pcmMaxSeconds keeps progressive PCM (48,000 bytes per second) inside the
	// 8 MiB per-utterance bound with headroom for a slower-than-estimated read.
	pcmMaxSeconds = 150
	// charactersPerSecond is a deliberately slow reading-rate estimate.
	charactersPerSecond = 11.0
	// stitchContextRunes bounds previous_text/next_text for split replies.
	stitchContextRunes = 300
)

// Options configure one worker session.
type Options struct {
	APIKey        string
	VoiceID       string
	ModelID       string
	BaseURL       string
	OutputFormat  string
	LanguageCode  string
	VoiceSettings *elevenlabs.VoiceSettings
	HTTPClient    *http.Client
	// RetrySleep and StreamIdleTimeout are test seams for the API client.
	RetrySleep        func(context.Context, time.Duration) error
	StreamIdleTimeout time.Duration
}

// Run speaks the worker protocol over reader/writer until EOF or shutdown.
func Run(reader io.Reader, writer io.Writer, options Options) error {
	if options.VoiceID == "" {
		options.VoiceID = DefaultVoiceID
	}
	if options.ModelID == "" {
		options.ModelID = DefaultModelID
	}
	if options.BaseURL == "" {
		options.BaseURL = DefaultBaseURL
	}
	options.BaseURL = strings.TrimRight(options.BaseURL, "/")
	if options.OutputFormat == "" {
		options.OutputFormat = OutputFormatAuto
	}

	s := &session{
		options: options,
		writer:  writer,
		queue:   make(chan protocol.Request, queueCapacity),
		profile: elevenlabs.ProfileFor(options.ModelID, nil),
	}
	s.setupErr = validateOptions(options)
	if s.setupErr == nil {
		s.client, s.setupErr = elevenlabs.New(elevenlabs.Options{
			BaseURL:           options.BaseURL,
			APIKey:            options.APIKey,
			HTTPClient:        options.HTTPClient,
			Sleep:             options.RetrySleep,
			StreamIdleTimeout: options.StreamIdleTimeout,
		})
	}

	workDone := make(chan struct{})
	go func() {
		defer close(workDone)
		s.workLoop()
	}()

	readErr := s.readLoop(reader)
	s.cancelAll()
	close(s.queue)
	<-workDone
	return readErr
}

type session struct {
	options Options
	client  *elevenlabs.Client

	writeMu sync.Mutex
	writer  io.Writer

	mu       sync.Mutex
	loaded   bool
	pending  int
	profile  elevenlabs.ModelProfile
	jobs     workerhost.Jobs
	setupErr error

	queue chan protocol.Request
}

func (s *session) readLoop(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var request protocol.Request
		if err := json.Unmarshal(line, &request); err != nil {
			s.sendError("", protocol.ErrorCodeInvalidRequest, "request is not valid JSON", false)
			continue
		}

		switch request.Type {
		case protocol.RequestHello:
			s.handleHello(request)
		case protocol.RequestHealth:
			s.send(s.healthResponse(request.ID))
		case protocol.RequestLoad:
			s.handleLoad(request)
		case protocol.RequestUnload:
			s.setLoaded(false)
			s.cancelAll()
			s.send(s.healthResponse(request.ID))
		case protocol.RequestCancel:
			s.jobs.Cancel(request.TargetID)
		case protocol.RequestShutdown:
			s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: request.ID})
			return nil
		case protocol.RequestSpeak:
			s.enqueue(request)
		default:
			message := fmt.Sprintf("elevenlabs worker cannot handle %q requests", request.Type)
			s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, message, false)
		}
	}
	return scanner.Err()
}

func (s *session) handleHello(request protocol.Request) {
	if request.ProtocolVersion != protocol.Version {
		message := fmt.Sprintf("protocol version %d is not supported; this worker speaks %d",
			request.ProtocolVersion, protocol.Version)
		s.sendError(request.ID, protocol.ErrorCodeProtocolMismatch, message, false)
		return
	}
	s.send(protocol.Response{
		Type:            protocol.ResponseHello,
		RequestID:       request.ID,
		ProtocolVersion: protocol.Version,
		Provider:        providerName,
		ProviderVersion: providerVersion,
		Role:            protocol.RoleTTS,
		Capabilities:    []string{"cancel", "load", "unload", "cloud", "stream"},
	})
}

// handleLoad checks the key and the selected model without spending credits.
// The model list doubles as the key check: it needs only the Models
// permission. A key restricted to speech cannot list models; that is not a
// bad key, so load succeeds and speech reports any problem itself.
func (s *session) handleLoad(request protocol.Request) {
	if s.setupErr != nil {
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, s.setupErr.Error(), false)
		return
	}
	if !s.client.HasKey() {
		s.sendError(request.ID, protocol.ErrorCodeMissingDependency,
			"ELEVENLABS_API_KEY is not set; add the API key in Settings â†’ Voice", false)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
	defer cancel()
	models, err := s.client.Models(ctx)
	switch {
	case err == nil:
		model, ok := findModel(models, s.options.ModelID)
		if !ok || !model.CanDoTextToSpeech {
			s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, unavailableModelMessage(s.options.ModelID, models), false)
			return
		}
		s.setProfile(elevenlabs.ProfileFor(s.options.ModelID, &model))
	case elevenlabs.IsKind(err, elevenlabs.KindPermission):
		s.setProfile(elevenlabs.ProfileFor(s.options.ModelID, nil))
	default:
		s.sendAPIError(request.ID, err)
		return
	}

	s.setLoaded(true)
	s.send(s.healthResponse(request.ID))
}

func findModel(models []elevenlabs.Model, id string) (elevenlabs.Model, bool) {
	for _, model := range models {
		if model.ID == id {
			return model, true
		}
	}
	return elevenlabs.Model{}, false
}

func unavailableModelMessage(id string, models []elevenlabs.Model) string {
	var speech []string
	for _, model := range models {
		if model.CanDoTextToSpeech && elevenlabs.ValidIdentifier(model.ID) {
			speech = append(speech, model.ID)
		}
	}
	sort.Strings(speech)
	message := fmt.Sprintf("ElevenLabs doesn't offer the speech model %q to this account; choose another model in Settings â†’ Voice", id)
	if len(speech) > 0 {
		if len(speech) > 8 {
			speech = speech[:8]
		}
		message += " (available: " + strings.Join(speech, ", ") + ")"
	}
	return message
}

func (s *session) enqueue(request protocol.Request) {
	if !s.jobs.Track(request.ID) {
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, "worker queue is full or the request ID is already queued", true)
		return
	}
	s.mu.Lock()
	s.pending++
	s.mu.Unlock()
	select {
	case s.queue <- request:
	default:
		s.jobs.Finish(request.ID)
		s.mu.Lock()
		s.pending--
		s.mu.Unlock()
		s.sendError(request.ID, protocol.ErrorCodeInternal, "elevenlabs worker queue is full", true)
	}
}

func (s *session) workLoop() {
	for request := range s.queue {
		s.mu.Lock()
		s.pending--
		canceled := s.jobs.Canceled(request.ID)
		loaded := s.loaded
		ctx := s.jobs.Context(request.ID)
		s.mu.Unlock()

		switch {
		case canceled:
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
		case !loaded:
			s.sendError(request.ID, protocol.ErrorCodeModelNotLoaded,
				"the ElevenLabs key has not been validated; send load first", true)
		default:
			s.speak(ctx, request)
		}

		s.jobs.Finish(request.ID)
	}
}

// speak sends the reply as one streaming request whenever it fits the
// model's per-request limit, so its delivery stays continuous and the first
// audio arrives as soon as ElevenLabs produces it. Longer text is split at
// paragraph or sentence boundaries and stitched with request context.
func (s *session) speak(ctx context.Context, request protocol.Request) {
	text, voiceID, ok := s.speechTarget(request)
	if !ok {
		return
	}
	plan := s.newSpeechPlan(text, request.Seed)
	pieces := elevenlabs.SplitForSpeech(text, plan.profile.MaxTextRunes)
	if len(pieces) == 0 {
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, "speak text is empty", false)
		return
	}

	seq, total := 0, 0
	var previousIDs []string
	for index := range pieces {
		if s.jobs.Canceled(request.ID) || ctx.Err() != nil {
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
			return
		}
		speech, err := s.streamPiece(ctx, voiceID, &plan, plan.pieceRequest(pieces, index, previousIDs), total == 0)
		if err != nil {
			if s.jobs.Canceled(request.ID) || ctx.Err() != nil {
				s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: request.ID})
				return
			}
			s.sendAPIError(request.ID, err)
			return
		}
		ok := s.relay(ctx, request.ID, speech.Body, plan.label, &seq, &total)
		_ = speech.Body.Close()
		if !ok {
			return
		}
		if speech.RequestID != "" {
			previousIDs = append(previousIDs, speech.RequestID)
		}
	}
	if total == 0 {
		s.sendError(request.ID, protocol.ErrorCodeInternal, elevenlabs.ErrNoAudio.Error(), true)
		return
	}
	s.send(protocol.Response{Type: protocol.ResponseDone, RequestID: request.ID})
}

// speechTarget validates the text and the optional per-request voice; false
// means an error frame was sent.
func (s *session) speechTarget(request protocol.Request) (string, string, bool) {
	text := strings.TrimSpace(request.Text)
	if len(text) > maxSpeechBytes {
		s.sendError(request.ID, protocol.ErrorCodeInvalidRequest,
			fmt.Sprintf("speak text exceeds %d KiB", maxSpeechBytes>>10), false)
		return "", "", false
	}
	voiceID := s.options.VoiceID
	if override := strings.TrimSpace(request.Voice); override != "" {
		if !elevenlabs.ValidIdentifier(override) {
			s.sendError(request.ID, protocol.ErrorCodeInvalidRequest, "the requested ElevenLabs voice ID is not valid", false)
			return "", "", false
		}
		voiceID = override
	}
	return text, voiceID, true
}

// speechPlan is everything about a reply's requests that does not change
// from one piece to the next, except the format after a fallback.
type speechPlan struct {
	profile  elevenlabs.ModelProfile
	modelID  string
	format   string
	label    string
	settings *elevenlabs.VoiceSettings
	language string
	seed     *uint32
}

func (s *session) newSpeechPlan(text string, seed *uint32) speechPlan {
	profile := s.currentProfile()
	format, label := s.outputFormat(text, profile)
	plan := speechPlan{
		profile: profile, modelID: s.options.ModelID, format: format, label: label,
		settings: profile.Shape(s.options.VoiceSettings), seed: seed,
	}
	if profile.LanguageCode {
		plan.language = s.options.LanguageCode
	}
	return plan
}

// pieceRequest builds one piece's request, stitched to its neighbors.
func (p speechPlan) pieceRequest(pieces []string, index int, previousIDs []string) elevenlabs.SpeechRequest {
	request := elevenlabs.SpeechRequest{
		Text:          pieces[index],
		ModelID:       p.modelID,
		OutputFormat:  p.format,
		LanguageCode:  p.language,
		VoiceSettings: p.settings,
		Seed:          p.seed,
	}
	if p.profile.RequestStitching && len(previousIDs) > 0 {
		request.PreviousRequestIDs = previousIDs
	} else if index > 0 {
		request.PreviousText = lastRunes(pieces[index-1], stitchContextRunes)
	}
	if index+1 < len(pieces) {
		request.NextText = firstRunes(pieces[index+1], stitchContextRunes)
	}
	return request
}

// streamPiece starts one piece. Before any audio of the reply has been sent,
// a refusal of the PCM format is answered by finishing the reply as MP3.
func (s *session) streamPiece(ctx context.Context, voiceID string, plan *speechPlan, request elevenlabs.SpeechRequest, firstAudio bool) (*elevenlabs.Speech, error) {
	speech, err := s.client.StreamSpeech(ctx, voiceID, request)
	if err == nil || !firstAudio || plan.format != pcmOutputFormat ||
		s.options.OutputFormat != OutputFormatAuto || !rejectsOutputFormat(err) {
		return speech, err
	}
	// PCM at 24 kHz is available on every plan today; if a plan or model
	// ever refuses it, finish the reply as MP3 instead of failing.
	plan.format, plan.label = "mp3_44100_128", "mp3"
	request.OutputFormat = plan.format
	return s.client.StreamSpeech(ctx, voiceID, request)
}

// relay forwards one response body as audio_chunk frames; false means the
// request already terminated (error or cancel frame sent).
func (s *session) relay(ctx context.Context, requestID string, body io.Reader, label string, seq, total *int) bool {
	buffer := make([]byte, chunkBytes)
	for {
		if s.jobs.Canceled(requestID) {
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: requestID})
			return false
		}
		// Relay whatever arrived: waiting to fill the buffer would hold back the
		// first audio of a reply.
		n, err := body.Read(buffer)
		if n > 0 {
			*total += n
			if *total > maxAudioBytes {
				s.sendError(requestID, protocol.ErrorCodeInternal,
					fmt.Sprintf("ElevenLabs audio exceeds %d MiB", maxAudioBytes>>20), false)
				return false
			}
			s.send(protocol.Response{
				Type:        protocol.ResponseAudioChunk,
				RequestID:   requestID,
				Seq:         *seq,
				AudioB64:    base64.StdEncoding.EncodeToString(buffer[:n]),
				AudioFormat: label,
			})
			*seq++
		}
		switch {
		case err == nil:
			continue
		case errors.Is(err, io.EOF):
			return true
		case s.jobs.Canceled(requestID) || ctx.Err() != nil:
			s.send(protocol.Response{Type: protocol.ResponseCanceled, RequestID: requestID})
			return false
		case errors.Is(err, elevenlabs.ErrStreamStalled):
			s.sendError(requestID, protocol.ErrorCodeInternal,
				"ElevenLabs stopped sending audio partway through the reply; try again", true)
			return false
		default:
			s.sendError(requestID, protocol.ErrorCodeInternal,
				"ElevenLabs audio stream was interrupted: "+s.client.Redact(err.Error()), true)
			return false
		}
	}
}

// outputFormat picks the wire format and the protocol label for a reply.
func (s *session) outputFormat(text string, profile elevenlabs.ModelProfile) (string, string) {
	if s.options.OutputFormat != OutputFormatAuto {
		return s.options.OutputFormat, formatLabel(s.options.OutputFormat)
	}
	speed := 1.0
	if settings := profile.Shape(s.options.VoiceSettings); settings != nil && settings.Speed != nil {
		speed = *settings.Speed
	}
	seconds := float64(utf8.RuneCountInString(text)) / (charactersPerSecond * speed)
	switch {
	case seconds <= pcmMaxSeconds:
		return pcmOutputFormat, pcmFormatLabel
	case seconds <= 480:
		return "mp3_44100_128", "mp3"
	default:
		return "mp3_44100_64", "mp3"
	}
}

// rejectsOutputFormat recognizes a refusal of the requested audio format, as
// opposed to a problem with the voice, text, key or plan in general.
func rejectsOutputFormat(err error) bool {
	apiErr, ok := elevenlabs.AsAPIError(err)
	if !ok {
		return false
	}
	switch apiErr.Kind() {
	case elevenlabs.KindInvalid, elevenlabs.KindPlan, elevenlabs.KindPermission:
	default:
		return false
	}
	signals := strings.ToLower(apiErr.Param + " " + apiErr.Code + " " + apiErr.Status + " " + apiErr.Message)
	return strings.Contains(signals, "output_format") || strings.Contains(signals, "output format")
}

func (s *session) sendAPIError(requestID string, err error) {
	apiErr, ok := elevenlabs.AsAPIError(err)
	if !ok {
		s.sendError(requestID, protocol.ErrorCodeInternal, s.client.Redact(err.Error()), true)
		return
	}
	code := protocol.ErrorCodeInternal
	switch apiErr.Kind() {
	case elevenlabs.KindAuth, elevenlabs.KindPermission, elevenlabs.KindPlan,
		elevenlabs.KindQuota, elevenlabs.KindUnusualActivity:
		code = protocol.ErrorCodeMissingDependency
	case elevenlabs.KindNotFound, elevenlabs.KindInvalid:
		code = protocol.ErrorCodeInvalidRequest
	}
	s.sendError(requestID, code, s.client.Redact(apiErr.Error()), apiErr.Retryable())
}

func validateOptions(options Options) error {
	if err := elevenlabs.ValidateBaseURL(options.BaseURL); err != nil {
		return err
	}
	if !elevenlabs.ValidIdentifier(options.VoiceID) {
		return errors.New("the ElevenLabs voice ID is not valid; choose a voice in Settings â†’ Voice")
	}
	if !elevenlabs.ValidIdentifier(options.ModelID) {
		return errors.New("the ElevenLabs model ID is not valid; choose a model in Settings â†’ Voice")
	}
	if options.LanguageCode != "" && !validLanguageCode(options.LanguageCode) {
		return errors.New("the ElevenLabs language code must be a two- or three-letter ISO 639 code")
	}
	switch format := options.OutputFormat; {
	case format == OutputFormatAuto, format == pcmOutputFormat,
		strings.HasPrefix(format, "mp3_") && len(format) <= 32,
		strings.HasPrefix(format, "opus_") && len(format) <= 32:
	default:
		// Other PCM rates and telephony codecs have no browser playback path.
		return fmt.Errorf("the ElevenLabs output format %q is not supported for playback", format)
	}
	return nil
}

func validLanguageCode(code string) bool {
	if len(code) < 2 || len(code) > 3 {
		return false
	}
	for _, r := range code {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func (s *session) cancelAll() { s.jobs.CancelAll() }

func (s *session) setLoaded(loaded bool) {
	s.mu.Lock()
	s.loaded = loaded
	s.mu.Unlock()
}

func (s *session) setProfile(profile elevenlabs.ModelProfile) {
	s.mu.Lock()
	s.profile = profile
	s.mu.Unlock()
}

func (s *session) currentProfile() elevenlabs.ModelProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profile
}

func (s *session) healthResponse(requestID string) protocol.Response {
	s.mu.Lock()
	loaded := s.loaded
	depth := s.pending
	s.mu.Unlock()

	state := protocol.ModelStateUnloaded
	if loaded {
		state = protocol.ModelStateReady
	}
	return protocol.Response{
		Type:       protocol.ResponseHealth,
		RequestID:  requestID,
		ModelState: state,
		QueueDepth: depth,
	}
}

func (s *session) sendError(requestID string, code string, message string, retryable bool) {
	s.send(protocol.Response{
		Type:      protocol.ResponseError,
		RequestID: requestID,
		Error: &protocol.WorkerError{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	})
}

func (s *session) send(response protocol.Response) {
	data, err := json.Marshal(response)
	if err != nil {
		return
	}
	data = append(data, '\n')
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = s.writer.Write(data)
}

func formatLabel(outputFormat string) string {
	switch {
	case outputFormat == pcmOutputFormat:
		return pcmFormatLabel
	case strings.HasPrefix(outputFormat, "mp3"):
		return "mp3"
	case strings.HasPrefix(outputFormat, "opus"):
		return "opus"
	default:
		return outputFormat
	}
}

func firstRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit])
}

func lastRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[len(runes)-limit:])
}
