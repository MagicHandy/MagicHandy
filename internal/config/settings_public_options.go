package config

func publicSettingsOptionHints() PublicSettingsOptionHints {
	return PublicSettingsOptionHints{
		HSPDispatchOwners: []string{
			DispatchOwnerCloudREST,
			DispatchOwnerBrowserBluetooth,
			DispatchOwnerIntiface,
		},
		APIApplicationIDSources: []string{
			ApplicationIDSourceBundled,
			ApplicationIDSourceDeveloperOverride,
		},
		MotionStyles: []string{
			MotionStyleGentle,
			MotionStyleBalanced,
			MotionStyleIntense,
		},
		HandyModels: []string{
			HandyModelOriginal,
			HandyModel2Standard,
			HandyModel2Pro,
		},
		AutopilotSpeechCadences: []string{
			AutopilotSpeechOff,
			AutopilotSpeechQuiet,
			AutopilotSpeechNatural,
			AutopilotSpeechTalkative,
			AutopilotSpeechCustom,
		},
		AutopilotMotionCadences: []string{
			AutopilotMotionScaled,
		},
		AutopilotAuthorities: []string{
			AutopilotSpeechMotionChatOnly,
			AutopilotSpeechMotionStyle,
			AutopilotSpeechMotionFull,
		},
		DiagnosticsVerbosities: []string{
			DiagnosticsVerbosityNormal,
			DiagnosticsVerbosityDebug,
			DiagnosticsVerbosityTrace,
		},
		LLMProviders:  []string{LLMProviderLlamaCPP, LLMProviderOllama},
		LlamaCPPModes: []string{LlamaCPPModeManaged, LlamaCPPModeExternal},
		LLMManagedLoadPolicies: []string{
			LLMManagedLoadStartup,
			LLMManagedLoadOnDemand,
		},
		LlamaCPPContextSizes: LlamaCPPContextSizes(),
		LLMReasoningModes: []string{
			LLMReasoningOff,
			LLMReasoningAuto,
		},
		LLMMaxOutputTokens: []int{128, 256, 512, 1024},
		LLMMotionModes:     []string{LLMMotionModeDynamic, LLMMotionModePattern, LLMMotionModeLayered, LLMMotionModeCreativeV2, LLMMotionModeOff},
		LLMChatVoices:      LLMChatVoices(),
		LLMUserAnatomies: []string{
			LLMUserAnatomyPenis,
			LLMUserAnatomyVagina,
			LLMUserAnatomyCustom,
		},
		LLMReactionStyles: LLMReactionStyles(),
		PromptSets: []string{
			PromptSetMagicHandyMotionV1,
			PromptSetMagicHandyMotionV1ES,
			PromptSetMagicHandyMotionV1PTBR,
			PromptSetMagicHandyMotionV1ZHHans,
			PromptSetMagicHandyMotionV1JA,
		},
		TTSProviders: voiceTTSProviderOptions(),
		ASRProviders: []string{
			VoiceProviderNone,
			VoiceASRProviderParakeet,
			VoiceASRProviderOpenAICompat,
			VoiceProviderCustom,
		},
		ParakeetSources: []string{
			ParakeetSourceApp,
			ParakeetSourceCustom,
		},
		TTSDevices:     voiceTTSDeviceOptions(),
		TTSTonePresets: TTSTonePresets(),
		ChatSpeechPolicies: []string{
			ChatSpeechInterrupt,
			ChatSpeechFinishCurrent,
		},
		ChatStartupBehaviors: []string{
			ChatStartupPrevious,
			ChatStartupNew,
		},
		Locales: []string{
			LocaleEnglish,
			LocaleSpanish,
			LocalePortugueseBrazil,
			LocaleSimplifiedChinese,
			LocaleJapanese,
		},
		Themes: SupportedUIThemes(),
	}
}

func voiceTTSProviderOptions() []string {
	return []string{
		VoiceProviderNone,
		VoiceTTSProviderElevenLabs,
		VoiceTTSProviderFasterQwen,
		VoiceTTSProviderChatterbox,
		VoiceTTSProviderOpenAICompat,
		VoiceProviderCustom,
	}
}

func voiceTTSDeviceOptions() []string {
	return []string{TTSDeviceAuto, TTSDeviceCUDA, TTSDeviceCPU}
}
