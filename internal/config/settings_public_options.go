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
		DiagnosticsVerbosities: []string{
			DiagnosticsVerbosityNormal,
			DiagnosticsVerbosityDebug,
			DiagnosticsVerbosityTrace,
		},
		LLMProviders: []string{
			LLMProviderLlamaCPP,
			LLMProviderOllama,
		},
		LlamaCPPModes: []string{
			LlamaCPPModeManaged,
			LlamaCPPModeExternal,
		},
		LLMReasoningModes: []string{
			LLMReasoningOff,
			LLMReasoningAuto,
		},
		LLMMaxOutputTokens: []int{128, 256, 512, 1024},
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
		TTSProviders: []string{
			VoiceProviderNone,
			VoiceTTSProviderElevenLabs,
			VoiceTTSProviderNeuTTSAir,
			VoiceProviderCustom,
		},
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
		NeuTTSSamplingModes: []string{
			NeuTTSSamplingFixed,
			NeuTTSSamplingRandom,
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
