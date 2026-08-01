package config

import (
	"errors"
	"fmt"
	"strings"
)

func applyMissingVoiceDefaults(settings, defaults VoiceSettings) VoiceSettings {
	// Settings version 1 originally had only raw worker paths and arguments.
	// Preserve those commands exactly by classifying them as custom providers.
	if settings.TTSProvider == "" {
		settings.TTSProvider = defaults.TTSProvider
		if settings.TTSWorkerPath != "" || len(settings.TTSWorkerArgs) > 0 {
			settings.TTSProvider = VoiceProviderCustom
		}
	}
	if settings.ASRProvider == "" {
		settings.ASRProvider = defaults.ASRProvider
		if settings.ASRWorkerPath != "" || len(settings.ASRWorkerArgs) > 0 {
			settings.ASRProvider = VoiceProviderCustom
		}
	}
	settings = applyMissingTTSDefaults(settings, defaults)
	settings = applyMissingASRDefaults(settings, defaults)
	if settings.InputMode == "" {
		settings.InputMode = defaults.InputMode
	}
	if settings.InputSensitivity == 0 {
		settings.InputSensitivity = defaults.InputSensitivity
	}
	if settings.InputSilenceMillis == 0 {
		settings.InputSilenceMillis = defaults.InputSilenceMillis
	}
	if settings.ChatSpeechPolicy == "" {
		settings.ChatSpeechPolicy = defaults.ChatSpeechPolicy
	}
	return settings
}

func applyMissingTTSDefaults(settings, defaults VoiceSettings) VoiceSettings {
	if settings.ElevenLabsVoiceID == "" {
		settings.ElevenLabsVoiceID = defaults.ElevenLabsVoiceID
	}
	if settings.ElevenLabsModelID == "" {
		settings.ElevenLabsModelID = defaults.ElevenLabsModelID
	}
	if settings.TTSBaseURL == "" {
		settings.TTSBaseURL = defaults.TTSBaseURL
	}
	if settings.TTSModel == "" {
		switch settings.TTSProvider {
		case VoiceTTSProviderFasterQwen:
			settings.TTSModel = DefaultFasterQwenModel
		case VoiceTTSProviderChatterbox:
			settings.TTSModel = DefaultChatterboxModel
		}
	}
	if settings.TTSVoice == "" {
		switch settings.TTSProvider {
		case VoiceTTSProviderFasterQwen:
			settings.TTSVoice = DefaultFasterQwenVoice
		case VoiceTTSProviderChatterbox:
			settings.TTSVoice = DefaultChatterboxVoice
		}
	}
	if settings.TTSResponseFormat == "" {
		settings.TTSResponseFormat = defaults.TTSResponseFormat
	}
	if settings.TTSHealthPath == "" {
		settings.TTSHealthPath = defaults.TTSHealthPath
	}
	if settings.TTSProvider == VoiceTTSProviderChatterbox &&
		(settings.TTSHealthPath == DefaultTTSHealthPath ||
			settings.TTSHealthPath == "/api/ui/initial-data") {
		settings.TTSHealthPath = DefaultChatterboxHealthPath
	}
	if settings.TTSServerPort == 0 {
		settings.TTSServerPort = defaults.TTSServerPort
	}
	if settings.TTSLanguage == "" {
		settings.TTSLanguage = defaults.TTSLanguage
	}
	if settings.TTSDevice == "" {
		settings.TTSDevice = defaults.TTSDevice
	}
	if settings.TTSSeedMode == "" {
		settings.TTSSeedMode = defaults.TTSSeedMode
		settings.TTSSeed = defaults.TTSSeed
	}
	if settings.TTSTonePreset == "" {
		settings.TTSTonePreset = defaults.TTSTonePreset
	}
	return settings
}

func applyMissingASRDefaults(settings, defaults VoiceSettings) VoiceSettings {
	if settings.ParakeetServerPort == 0 {
		settings.ParakeetServerPort = defaults.ParakeetServerPort
	}
	if settings.ParakeetSource == "" {
		settings.ParakeetSource = defaults.ParakeetSource
		if settings.ParakeetServerPath != "" || settings.ParakeetModelPath != "" {
			settings.ParakeetSource = ParakeetSourceCustom
		}
	}
	return settings
}

func normalizeVoiceStrings(settings VoiceSettings) VoiceSettings {
	settings.TTSProvider = strings.TrimSpace(settings.TTSProvider)
	settings.ASRProvider = strings.TrimSpace(settings.ASRProvider)
	settings.TTSWorkerPath = strings.TrimSpace(settings.TTSWorkerPath)
	settings.ASRWorkerPath = strings.TrimSpace(settings.ASRWorkerPath)
	settings.TTSWorkerArgs = trimVoiceArgs(settings.TTSWorkerArgs)
	settings.ASRWorkerArgs = trimVoiceArgs(settings.ASRWorkerArgs)
	settings.ElevenLabsVoiceID = strings.TrimSpace(settings.ElevenLabsVoiceID)
	settings.ElevenLabsModelID = strings.TrimSpace(settings.ElevenLabsModelID)
	settings.TTSBaseURL = strings.TrimRight(strings.TrimSpace(settings.TTSBaseURL), "/")
	settings.TTSModel = strings.TrimSpace(settings.TTSModel)
	settings.TTSVoice = strings.TrimSpace(settings.TTSVoice)
	settings.TTSResponseFormat = strings.ToLower(strings.TrimSpace(settings.TTSResponseFormat))
	settings.TTSHealthPath = strings.TrimSpace(settings.TTSHealthPath)
	if settings.TTSHealthPath != "" && !strings.HasPrefix(settings.TTSHealthPath, "/") {
		settings.TTSHealthPath = "/" + settings.TTSHealthPath
	}
	settings.TTSModuleRoot = strings.TrimSpace(settings.TTSModuleRoot)
	settings.TTSReferenceWAV = strings.TrimSpace(settings.TTSReferenceWAV)
	settings.TTSReferenceText = strings.TrimSpace(settings.TTSReferenceText)
	settings.TTSLanguage = strings.TrimSpace(settings.TTSLanguage)
	settings.TTSDevice = strings.ToLower(strings.TrimSpace(settings.TTSDevice))
	settings.TTSSeedMode = strings.ToLower(strings.TrimSpace(settings.TTSSeedMode))
	settings.TTSTonePreset = strings.ToLower(strings.TrimSpace(settings.TTSTonePreset))
	settings.TTSTonePrompt = strings.TrimSpace(settings.TTSTonePrompt)
	settings.ChatSpeechPolicy = strings.ToLower(strings.TrimSpace(settings.ChatSpeechPolicy))
	settings.ParakeetServerPath = strings.TrimSpace(settings.ParakeetServerPath)
	settings.ParakeetModelPath = strings.TrimSpace(settings.ParakeetModelPath)
	settings.ParakeetSource = strings.TrimSpace(settings.ParakeetSource)
	settings.ASRBaseURL = strings.TrimRight(strings.TrimSpace(settings.ASRBaseURL), "/")
	settings.ASRModel = strings.TrimSpace(settings.ASRModel)
	settings.InputMode = strings.TrimSpace(settings.InputMode)
	return settings
}

func validateVoiceSettings(settings VoiceSettings) error {
	if !oneOf(settings.ChatSpeechPolicy, ChatSpeechInterrupt, ChatSpeechFinishCurrent) {
		return fmt.Errorf("unknown chat speech policy %q", settings.ChatSpeechPolicy)
	}
	if err := validateVoiceProviders(settings); err != nil {
		return err
	}
	if err := validateTTSSettings(settings); err != nil {
		return err
	}
	if err := validateASRSettings(settings); err != nil {
		return err
	}
	return validateVoiceInputSettings(settings)
}

func validateVoiceProviders(settings VoiceSettings) error {
	if !oneOf(settings.TTSProvider, VoiceProviderNone, VoiceTTSProviderElevenLabs,
		VoiceTTSProviderFasterQwen, VoiceTTSProviderChatterbox,
		VoiceTTSProviderOpenAICompat, VoiceProviderCustom) {
		return fmt.Errorf("unknown TTS provider %q", settings.TTSProvider)
	}
	if !oneOf(settings.ASRProvider, VoiceProviderNone, VoiceASRProviderParakeet, VoiceASRProviderOpenAICompat, VoiceProviderCustom) {
		return fmt.Errorf("unknown ASR provider %q", settings.ASRProvider)
	}
	return nil
}

func validateTTSSettings(settings VoiceSettings) error {
	if oneOf(settings.TTSProvider, VoiceTTSProviderFasterQwen,
		VoiceTTSProviderChatterbox, VoiceTTSProviderOpenAICompat) {
		if settings.TTSBaseURL == "" {
			return errors.New("TTS base URL is required")
		}
		if err := validateLLMBaseURL("TTS", settings.TTSBaseURL); err != nil {
			return err
		}
	}
	if err := validateTTSFieldBounds(settings); err != nil {
		return err
	}
	if err := validateTTSResponseFormat(settings.TTSProvider, settings.TTSResponseFormat); err != nil {
		return err
	}
	if settings.TTSHealthPath == "" || !strings.HasPrefix(settings.TTSHealthPath, "/") ||
		strings.ContainsAny(settings.TTSHealthPath, "?#\r\n") {
		return errors.New("TTS health path must be an absolute path without query or fragment")
	}
	if !oneOf(settings.TTSDevice, TTSDeviceAuto, TTSDeviceCUDA, TTSDeviceCPU) {
		return fmt.Errorf("unknown TTS device %q", settings.TTSDevice)
	}
	if !oneOf(settings.TTSSeedMode, TTSSeedModeFixed, TTSSeedModeVaried) {
		return fmt.Errorf("unknown TTS seed mode %q", settings.TTSSeedMode)
	}
	if !oneOf(settings.TTSTonePreset, TTSTonePresets()...) {
		return fmt.Errorf("unknown TTS tone preset %q", settings.TTSTonePreset)
	}
	if settings.TTSProvider == VoiceTTSProviderFasterQwen &&
		settings.TTSTonePreset == TTSToneCustom && settings.TTSTonePrompt == "" {
		return errors.New("custom TTS tone requires a prompt")
	}
	if settings.TTSProvider == VoiceTTSProviderFasterQwen &&
		!oneOf(settings.TTSDevice, TTSDeviceAuto, TTSDeviceCUDA) {
		return errors.New("faster Qwen3-TTS requires an NVIDIA CUDA device")
	}
	if err := validateChatterboxVoice(settings.TTSProvider, settings.TTSVoice); err != nil {
		return err
	}
	return nil
}

// validateTTSFieldBounds covers the size and range limits, which are independent
// of which provider is selected.
func validateTTSFieldBounds(settings VoiceSettings) error {
	if len(settings.ElevenLabsVoiceID) > 256 || len(settings.ElevenLabsModelID) > 256 ||
		len(settings.TTSModel) > 512 || len(settings.TTSVoice) > 512 || len(settings.ASRModel) > 256 {
		return errors.New("voice and model identifiers exceed their maximum length")
	}
	if len(settings.TTSReferenceText) > 8<<10 {
		return errors.New("TTS reference transcript must not exceed 8 KiB")
	}
	if len(settings.TTSTonePrompt) > 2<<10 {
		return errors.New("TTS tone prompt must not exceed 2 KiB")
	}
	if settings.TTSServerPort < 1 || settings.TTSServerPort > 65535 {
		return errors.New("TTS server port must be between 1 and 65535")
	}
	return nil
}

// TTSTonePresets returns the backend-authoritative tone choices in UI order.
func TTSTonePresets() []string {
	return []string{
		TTSToneNatural,
		TTSToneWarm,
		TTSTonePlayful,
		TTSToneTender,
		TTSToneCommanding,
		TTSToneExcited,
		TTSToneCustom,
	}
}

// ttsDeliveryFraming closes every built-in tone preset. An instruction-following
// TTS model reads a bare emotion adjective as a cue to act the emotion out, which
// is why the presets used to arrive as caricature: "excited" became shouting and
// "commanding" became an announcer. Each preset now names the mechanics that
// produce the tone -- pace, pitch movement, mic distance, timing -- and this
// shared clause asks for those mechanics delivered straight.
//
// It used to end "not a performance or an announcement", which Commanding had to
// opt out of because backing off was that preset's own failure mode. Commanding
// no longer earns its authority from volume, so the clause stops working against
// it, and one shared framing serves all five again.
const ttsDeliveryFraming = " Sound like a real person speaking to one listener, not a performance."

// ttsEaseAnchor closes every built-in preset, and is the single most load-bearing
// clause here. Whatever a preset asks for has to be held across a whole reply,
// and the failures reported from real use -- Commanding and Tender straining,
// Warm turning shouty, Excited going nasal -- were all the voice being driven
// past what it can sustain. Naming the ceiling explicitly, and saying it applies
// all the way through, is what keeps the delivery inside it.
const ttsEaseAnchor = " Keep the voice relaxed and comfortable the whole way through, never pushed, strained, or louder than it needs to be."

// Three rules hold across every preset below, learned from Commanding arriving
// in an audibly foreign accent on one seed and sounding timid on the rest.
//
// Never ask for level or flat pitch. English declaratives close on a falling
// contour; a level close is the prosody of a syllable-timed language, so asking
// for one pushes a multilingual model toward accented English and can land there
// outright on an unlucky seed. It also costs the tone its conviction, since a
// sentence that never resolves downward sounds tentative. Where a preset needs
// to rule out uptalk, the instruction is "falling", never "flat".
//
// Never ask for loose, slurred, or reduced articulation. Consonant precision is
// one of the strongest accent cues a synthesizer has, so relaxing it invites the
// same drift. Lightness belongs in pace and pitch, not in diction.
//
// Never make the word the prosodic unit either, which is the same mistake from
// the other side. Excited asked to keep "every word clearly articulated rather
// than rushed together" and got exactly that: every word released separately,
// with an abrupt stop at the end of each and a thin, nasal quality from the
// sustained effort. English runs words together inside a phrase, and a preset
// that forbids it buys careful diction at the cost of sounding synthetic. Aim
// articulation at the phrase, and let the words connect within it.
//
// Shape the pitch range rather than moving the whole voice. "Lifted pitch" or
// "low pitch" as a baseline shift changes the apparent speaker -- raising it thins
// the voice toward sounding younger, which is its own version of the timidity
// problem. Ask for movement within the range instead: wider on stressed words,
// gentler across a phrase.
//
// Do not stack the reducers. Quiet, slow, low, and falling all push the voice
// the same direction, and the bottom of that stack is where phonation gives out
// into press or creak -- which is heard as straining. Tender asked for softly
// AND slowly AND low volume AND audible breath AND a falling close, with nothing
// holding the voice up, and strained. Warm survives the same direction because
// it reduces on fewer axes and says "relaxed and unforced" outright. So a preset
// that asks for a quiet or slow delivery has to pair it with a phonation cue
// that keeps the voice supported, and leave the bottom of the range unused.
//
// Above all, keep each preset short. Every clause is a constraint the model must
// satisfy simultaneously and hold for the length of the utterance, so a preset
// that stacks five or six of them leaves only an extreme corner of the model's
// range to satisfy them all in, and extreme corners are where the artifacts are.
// This is what made the earlier presets test clean and fail in use: the preview
// button speaks four words, over which there is barely one intonation contour to
// get wrong, while a real reply is several sentences that the demands have to
// survive. One defining mechanic, one contour rule, and the shared ease anchor
// is the whole budget. Anything more belongs in a Custom prompt.

// ResolveTTSTonePrompt maps a saved preset to the instruction sent only to
// instruction-capable TTS providers. Natural intentionally resolves empty so
// existing Faster Qwen installations retain their previous behavior.
func ResolveTTSTonePrompt(settings VoiceSettings) string {
	switch settings.TTSTonePreset {
	case TTSToneWarm:
		return "Speak quietly and close to the microphone, unhurried and easy, letting sentences settle downward at the end." + ttsEaseAnchor + ttsDeliveryFraming
	case TTSTonePlayful:
		return "Speak at an easy conversational pace with a light smile in the voice, varying the timing: linger on a word, then move lightly through the next few." + ttsEaseAnchor + ttsDeliveryFraming
	case TTSToneTender:
		return "Speak gently and a little more slowly, close to the microphone, letting sentences settle softly downward without sinking to the bottom of your range." + ttsEaseAnchor + ttsDeliveryFraming
	case TTSToneCommanding:
		return "Speak evenly and unhurried, with a calm certainty that comes from steadiness rather than force, letting each sentence resolve downward with quiet finality." + ttsEaseAnchor + ttsDeliveryFraming
	case TTSToneExcited:
		return "Speak with quick, lively energy and wide pitch movement, keeping the voice open and the words connected into flowing phrases, and still letting sentences resolve downward at the end." + ttsEaseAnchor + ttsDeliveryFraming
	case TTSToneCustom:
		return strings.TrimSpace(settings.TTSTonePrompt)
	default:
		return ""
	}
}

func validateChatterboxVoice(provider, voice string) error {
	if provider == VoiceTTSProviderChatterbox && !validChatterboxVoiceName(voice) {
		return errors.New("chatterbox voice must be a plain .wav file name")
	}
	return nil
}

func validChatterboxVoiceName(value string) bool {
	return len(value) > len(".wav") &&
		strings.HasSuffix(strings.ToLower(value), ".wav") &&
		!strings.ContainsAny(value, `\/<>:"|?*`+"\r\n") &&
		strings.TrimRight(value, ". ") == value
}

func validateTTSResponseFormat(provider, format string) error {
	if !oneOf(format, "wav", "mp3", "opus", "aac", "flac") {
		return fmt.Errorf("unknown TTS response format %q", format)
	}
	if provider == VoiceTTSProviderFasterQwen && format != "wav" {
		return fmt.Errorf("faster Qwen3-TTS does not support %q output", format)
	}
	if provider == VoiceTTSProviderChatterbox && !oneOf(format, "wav", "mp3", "opus") {
		return fmt.Errorf("chatterbox does not support %q output", format)
	}
	return nil
}

func validateASRSettings(settings VoiceSettings) error {
	if settings.ASRProvider == VoiceASRProviderOpenAICompat {
		if settings.ASRBaseURL == "" {
			return errors.New("ASR base URL is required")
		}
		if err := validateLLMBaseURL("ASR", settings.ASRBaseURL); err != nil {
			return err
		}
	}
	if settings.ParakeetServerPort < 1 || settings.ParakeetServerPort > 65535 {
		return errors.New("parakeet server port must be between 1 and 65535")
	}
	if !oneOf(settings.ParakeetSource, ParakeetSourceApp, ParakeetSourceCustom) {
		return fmt.Errorf("unknown Parakeet source %q", settings.ParakeetSource)
	}
	return nil
}

func validateVoiceInputSettings(settings VoiceSettings) error {
	if !oneOf(settings.InputMode, VoiceInputModeHandsFree, VoiceInputModeHold) {
		return fmt.Errorf("unknown voice input mode %q", settings.InputMode)
	}
	if settings.InputSensitivity < 1 || settings.InputSensitivity > 100 {
		return errors.New("voice input sensitivity must be between 1 and 100")
	}
	if settings.InputSilenceMillis < 300 || settings.InputSilenceMillis > 3000 {
		return errors.New("voice input silence delay must be between 300 and 3000 milliseconds")
	}
	return nil
}

func trimVoiceArgs(args []string) []string {
	trimmed := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			trimmed = append(trimmed, arg)
		}
	}
	if len(trimmed) == 0 {
		return nil
	}
	return trimmed
}
