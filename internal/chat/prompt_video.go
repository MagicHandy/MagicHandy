package chat

// State the route to chat control in the reply's language, without a canned
// reply that can replace its voice. The source selector and opening the chat
// panel are distinct controls. Merely choosing a source does not start motion.
func videoControlInstructions(locale promptLocale) string {
	switch locale {
	case promptLocaleSpanish:
		return `Si la persona pide cambiar el movimiento, indica que debe elegir «Chat» en el selector de fuente de movimiento junto al video. Abrir el panel de chat no cambia la fuente. Elegir Chat no inicia el dispositivo ni anula los ajustes o permisos guardados. Explica brevemente este límite en la voz seleccionada, sin prometer tomar el control.`
	case promptLocalePortugueseBrazil:
		return `Se a pessoa pedir para mudar o movimento, indique que deve selecionar «Chat» na fonte de movimento ao lado do vídeo. Abrir o painel de chat não muda a fonte. Selecionar Chat não inicia o dispositivo nem substitui as configurações ou permissões salvas. Explique esse limite brevemente na voz selecionada, sem prometer assumir o controle.`
	case promptLocaleSimplifiedChinese:
		return `如果用户要求改变设备动作，请说明需要在视频旁的动作来源选择器中选择“聊天（Chat）”。仅打开聊天面板不会改变动作来源。选择 Chat 不会启动设备，也不会绕过已保存的设置或控制权限。用所选语气简要说明限制，不要承诺立即接管设备。`
	case promptLocaleJapanese:
		return `動作の変更を求められたら、動画の横にあるモーションの操作元で「チャット（Chat）」を選ぶ必要があると説明してください。チャット欄を開くだけでは操作元は変わりません。Chat を選ぶだけでは機器は動き始めず、保存された設定や操作権限も引き続き適用されます。選択された口調で制限を短く説明し、すぐに操作を引き継ぐとは約束しないでください。`
	default:
		return `If asked to change motion, direct the person to choose Chat in the motion source selector beside the video. Opening the chat panel does not change the source. Choosing Chat does not start the device or override saved settings or control permissions. Explain this briefly in the selected voice and language without promising to take control.`
	}
}
