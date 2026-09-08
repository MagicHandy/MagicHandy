package httpapi

import (
	"errors"
	"path/filepath"
	"strconv"

	"github.com/mapledaemon/MagicHandy/internal/config"
)

func (m *setupManager) StartVoiceUpdate(previous config.VoiceSettings) (setupJob, error) {
	var id string
	for _, module := range setupVoiceModules {
		if module.Provider == previous.TTSProvider {
			id = module.ID
		}
	}
	module, request, err := m.validateVoiceInstall(setupVoiceInstallRequest{
		Module: id, Device: previous.TTSDevice, AutoLaunch: previous.TTSAutoLaunch, updateFrom: &previous,
	})
	if err != nil {
		return setupJob{}, err
	}
	module.Port = previous.TTSServerPort
	ctx, job, err := m.reserveJob("tts_update", module.ID, request.Device, "TTS module update queued.")
	if err != nil {
		return setupJob{}, err
	}
	m.wg.Add(1)
	go m.runVoiceInstall(ctx, job.ID, module, request)
	return job, nil
}

func voiceInstallArguments(script, dataDir, defaultRoot string, module setupVoiceModule, request setupVoiceInstallRequest) (string, []string, error) {
	root := defaultRoot
	if request.updateFrom != nil {
		root = voiceModuleHome(ttsModuleRoot(*request.updateFrom, dataDir))
		if root == "" {
			return "", nil, errors.New("the installed TTS module root is unavailable")
		}
	}
	arguments := []string{
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script,
		"-Module", module.ID, "-DataDir", dataDir, "-InstallRoot", root,
		"-Device", request.Device, "-Port", strconv.Itoa(module.Port),
		"-Yes", "-SkipAppConfiguration",
	}
	if request.AutoLaunch {
		arguments = append(arguments, "-AutoLaunch")
	}
	if previous := request.updateFrom; previous != nil {
		arguments = append(arguments, "-Update", "-Model", previous.TTSModel, "-Voice", previous.TTSVoice, "-Language", previous.TTSLanguage)
		if previous.SpeakReplies {
			arguments = append(arguments, "-SpeakReplies")
		}
		if module.Provider == config.VoiceTTSProviderChatterbox {
			oldRoot := ttsModuleRoot(*previous, dataDir)
			voicePath := filepath.Join(oldRoot, "runtime", "voices", previous.TTSVoice)
			if !isRegularFileWithoutLinks(oldRoot, voicePath) {
				return "", nil, errors.New("the selected Chatterbox voice is unavailable; restore it before updating")
			}
			arguments = append(arguments, "-ReferenceWav", voicePath)
		}
	}
	return root, arguments, nil
}

func applyVoiceModuleUpdate(current config.VoiceSettings, result setupVoiceInstallResult) (config.VoiceSettings, error) {
	previous := result.updateFrom
	if previous == nil || current.TTSProvider != previous.TTSProvider ||
		current.TTSModuleRoot != previous.TTSModuleRoot || current.TTSModel != previous.TTSModel ||
		current.TTSVoice != previous.TTSVoice || current.TTSDevice != previous.TTSDevice || current.TTSServerPort != previous.TTSServerPort {
		return current, errors.New("TTS selection changed during the update; the prepared runtime was retained without activation")
	}
	// Preserve current voice enablement, reference conditioning, seed/tone,
	// language, model, custom port, and every unrelated ASR/credential field.
	current.TTSModuleRoot = result.Root
	return current, nil
}
