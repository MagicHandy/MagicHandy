# Localization Wording

## Purpose

This is the canonical source-of-truth for every user-facing string in
MagicHandy, with translations in **English (en)**, **Spanish (es)**,
**Portuguese (pt-BR)**, **Simplified Chinese (zh-Hans)**, and **Japanese
(ja)**. It exists so translation is done once, consistently, before any i18n
framework is wired in (there is none yet — the UI ships hardcoded English).

When a string in `web/` or `internal/chat` changes, update the matching row
here in the same PR, the way `docs/goal-scorecard.md` tracks goals.

## Content Boundary (read this first)

MagicHandy controls an intimate adult device. Localization must not turn that
purpose into a blanket rule that shipped wording stays non-explicit.

- **Functional UI** (buttons, labels, status, errors, settings) should stay
  neutral and precise unless the feature itself names adult anatomy or an adult
  mode. Translate these rows directly; do not add sexual tone where the English
  is operational.
- **Prompt, persona, anatomy, memory, and voice-output text may be explicit.**
  StrokeGPT-ReVibed's actual default chat prompt told the model to be an
  adult erotic partner, use direct erotic language when it fits, preserve
  explicit wording, and use anatomy-specific terms. Do not sanitize or
  euphemize those strings during localization.
- **User-authored prompt sets and memories are first-class content.** Their
  text must be preserved verbatim except for explicit user-requested edits or
  translations. A localization layer must never silently rewrite explicit
  sexual language into clinical wording.
- **Prompt packs are separate from functional catalogs.** This document
  catalogs functional UI text and starter prompt text. Full legacy prompt
  parity and adult-language rules live in
  `docs/stgpt-rv-prompt-inventory.md`.

Rule of thumb: neutral UI is fine; adult prompt behavior is also fine. The
mistake to avoid is a global "non-explicit only" policy, because it would make
MagicHandy worse at its core use case.

## Glossary (keep these consistent everywhere)

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Motion | Movimiento | Movimento | 运动 | モーション |
| Speed | Velocidad | Velocidade | 速度 | 速度 |
| Stroke range | Recorrido | Curso | 行程 | ストローク範囲 |
| Reverse direction | Invertir dirección | Inverter direção | 反转方向 | 方向を反転 |
| Transport | Transporte | Transporte | 传输方式 | トランスポート |
| Dispatch owner | Origen de envío | Origem de envio | 指令来源 | 送信元 |
| Pattern | Patrón | Padrão | 模式 | パターン |
| Program | Programa | Programa | 程序 | プログラム |
| Funscript | Funscript | Funscript | Funscript | Funscript |
| Trim | Recortar | Recortar | 裁剪 | トリミング |
| Timeline | Línea de tiempo | Linha do tempo | 时间轴 | タイムライン |
| Selection | Selección | Seleção | 选择范围 | 選択範囲 |
| Freestyle | Modo libre | Modo livre | 自由模式 | フリースタイル |
| Pause / Resume | Pausar / Reanudar | Pausar / Retomar | 暂停 / 继续 | 一時停止 / 再開 |
| Stop | Detener | Parar | 停止 | 停止 |
| Prompt set | Conjunto de prompts | Conjunto de prompts | 提示词集 | プロンプトセット |
| Memory | Memoria | Memória | 记忆 | メモリ |
| Diagnostics | Diagnóstico | Diagnóstico | 诊断 | 診断 |
| Connection key | Clave de conexión | Chave de conexão | 连接密钥 | 接続キー |

Motion pattern names (mild, kept short):

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Stroke | Vaivén | Vaivém | 往复 | ストローク |
| Pulse | Pulso | Pulso | 脉冲 | パルス |
| Tease | Provocación | Provocação | 挑逗 | じらし |

## Persistent Control Bar

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| MagicHandy | MagicHandy | MagicHandy | MagicHandy | MagicHandy |
| Checking core | Comprobando núcleo | Verificando núcleo | 正在检查核心 | コアを確認中 |
| Core online | Núcleo en línea | Núcleo on-line | 核心在线 | コア接続済み |
| Core unavailable | Núcleo no disponible | Núcleo indisponível | 核心不可用 | コア利用不可 |
| Transport pending | Transporte pendiente | Transporte pendente | 传输待定 | トランスポート待機中 |
| Controller pending | Controlador pendiente | Controlador pendente | 控制端待定 | コントローラー待機中 |
| Controller active | Controlador activo | Controlador ativo | 控制端已激活 | コントローラー有効 |
| Read-only | Solo lectura | Somente leitura | 只读 | 読み取り専用 |
| Live motion state (aria) | Estado de movimiento en vivo | Estado do movimento ao vivo | 实时运动状态 | リアルタイムのモーション状態 |
| Idle | Inactivo | Inativo | 空闲 | 待機中 |
| Running | En marcha | Em execução | 运行中 | 動作中 |
| Paused | En pausa | Em pausa | 已暂停 | 一時停止中 |
| Unavailable | No disponible | Indisponível | 不可用 | 利用不可 |
| Stop everything | Detener todo | Parar tudo | 全部停止 | すべて停止 |
| Emergency stop all motion (aria) | Parada de emergencia de todo el movimiento | Parada de emergência de todo o movimento | 紧急停止所有运动 | すべてのモーションを緊急停止 |
| Open settings (aria) | Abrir ajustes | Abrir configurações | 打开设置 | 設定を開く |
| Core connection lost | Se perdió la conexión con el núcleo | Conexão com o núcleo perdida | 与核心的连接已断开 | コアとの接続が切断されました |
| Backend-required controls are locked until the core responds. | Los controles que requieren el servicio están bloqueados hasta que el núcleo responda. | Os controles que exigem o serviço ficam bloqueados até o núcleo responder. | 需要后端的控件将被锁定，直到核心恢复响应。 | コアが応答するまで、バックエンドを必要とする操作はロックされます。 |

## Chat — Autopilot

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Autopilot | Autopilot | Autopilot | 自动驾驶 | Autopilot |
| Start Autopilot | Iniciar Autopilot | Iniciar Autopilot | 启动自动驾驶 | Autopilotを開始 |
| Stop Autopilot | Detener Autopilot | Parar Autopilot | 停止自动驾驶 | Autopilotを停止 |
| Pause Autopilot | Pausar Autopilot | Pausar Autopilot | 暂停自动驾驶 | Autopilotを一時停止 |
| Resume Autopilot | Reanudar Autopilot | Retomar Autopilot | 继续自动驾驶 | Autopilotを再開 |
| Off | Desactivado | Desativado | 关闭 | オフ |
| Active | Activo | Ativo | 运行中 | 実行中 |
| Starting | Iniciando | Iniciando | 正在启动 | 開始中 |
| Stopping | Deteniendo | Parando | 正在停止 | 停止中 |
| Pausing | Pausando | Pausando | 正在暂停 | 一時停止中 |
| Resuming | Reanudando | Retomando | 正在继续 | 再開中 |
| Paused | En pausa | Em pausa | 已暂停 | 一時停止 |
| Choosing first segment | Eligiendo el primer segmento | Escolhendo o primeiro segmento | 正在选择第一个片段 | 最初のセグメントを選択中 |
| Assistant selected | Seleccionado por el asistente | Selecionado pelo assistente | 助手已选择 | アシスタントが選択 |
| Planner fallback | Alternativa del planificador | Alternativa do planejador | 规划器回退 | プランナーのフォールバック |
| Continuing current pattern | Manteniendo el patrón actual | Mantendo o padrão atual | 继续当前模式 | 現在のパターンを継続 |
| Motion has not started | El movimiento aún no ha comenzado | O movimento ainda não começou | 运动尚未开始 | モーションはまだ開始されていません |
| Autopilot started. | Autopilot iniciado. | Autopilot iniciado. | 自动驾驶已启动。 | Autopilotを開始しました。 |
| Autopilot stopped. | Autopilot detenido. | Autopilot parado. | 自动驾驶已停止。 | Autopilotを停止しました。 |
| Motion paused. | Movimiento pausado. | Movimento pausado. | 运动已暂停。 | モーションを一時停止しました。 |
| Motion resumed. | Movimiento reanudado. | Movimento retomado. | 运动已继续。 | モーションを再開しました。 |

## Sidebar — Controls

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Controls | Controles | Controles | 控制 | 操作 |
| Start Freestyle | Iniciar modo libre | Iniciar modo livre | 开始自由模式 | フリースタイル開始 |
| Stop Freestyle | Detener modo libre | Parar modo livre | 停止自由模式 | フリースタイル停止 |
| Pause | Pausar | Pausar | 暂停 | 一時停止 |
| Resume | Reanudar | Retomar | 继续 | 再開 |
| Chat keeps motion going | El chat mantiene el movimiento | O chat mantém o movimento | 聊天保持运动 | チャットでモーションを継続 |
| restarts after connection recovery only | solo se reanuda tras recuperar la conexión | reinicia apenas após recuperar a conexão | 仅在连接恢复后重启 | 接続復旧後のみ再開します |
| Chat keepalive on. | Continuación por chat activada. | Continuação por chat ativada. | 聊天保活已开启。 | チャット継続をオンにしました。 |
| Chat keepalive off. | Continuación por chat desactivada. | Continuação por chat desativada. | 聊天保活已关闭。 | チャット継続をオフにしました。 |
| Freestyle running. | Modo libre en marcha. | Modo livre em execução. | 自由模式运行中。 | フリースタイル動作中。 |
| Freestyle stopped. | Modo libre detenido. | Modo livre parado. | 自由模式已停止。 | フリースタイルを停止しました。 |

## Sidebar — Quick Settings

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Quick settings | Ajustes rápidos | Ajustes rápidos | 快捷设置 | クイック設定 |
| applies immediately | se aplica al instante | aplica-se imediatamente | 立即生效 | 即時反映 |
| Speed min | Velocidad mín. | Velocidade mín. | 最低速度 | 最低速度 |
| Speed max | Velocidad máx. | Velocidade máx. | 最高速度 | 最高速度 |
| Stroke min | Recorrido mín. | Curso mín. | 最小行程 | 最小ストローク |
| Stroke max | Recorrido máx. | Curso máx. | 最大行程 | 最大ストローク |
| Reverse direction | Invertir dirección | Inverter direção | 反转方向 | 方向を反転 |
| Style | Estilo | Estilo | 风格 | スタイル |
| biases autonomous pacing | ajusta el ritmo autónomo | ajusta o ritmo autônomo | 影响自主节奏 | 自律動作のペースに影響 |
| Gentle | Suave | Suave | 轻柔 | ソフト |
| Balanced | Equilibrado | Equilibrado | 均衡 | バランス |
| Intense | Intenso | Intenso | 强烈 | ハード |
| Applied | Aplicado | Aplicado | 已应用 | 反映しました |
| Applying… | Aplicando… | Aplicando… | 正在应用… | 反映中… |

## Sidebar — Manual Motion (test)

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Manual motion | Movimiento manual | Movimento manual | 手动运动 | 手動モーション |
| testing (badge) | prueba | teste | 测试 | テスト |
| Drives the device directly to test the connection. Normal motion comes from chat. | Controla el dispositivo directamente para probar la conexión. El movimiento normal proviene del chat. | Controla o dispositivo diretamente para testar a conexão. O movimento normal vem do chat. | 直接驱动设备以测试连接。正常运动来自聊天。 | 接続テスト用にデバイスを直接動かします。通常のモーションはチャットから行います。 |
| Start test | Iniciar prueba | Iniciar teste | 开始测试 | テスト開始 |
| Stop test | Detener prueba | Parar teste | 停止测试 | テスト停止 |
| Pattern | Patrón | Padrão | 模式 | パターン |
| Speed | Velocidad | Velocidade | 速度 | 速度 |
| Motion unavailable | Movimiento no disponible | Movimento indisponível | 运动不可用 | モーション利用不可 |

## Chat

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Local Chat | Chat local | Chat local | 本地聊天 | ローカルチャット |
| Message MagicHandy — chat can start, adjust, and stop motion | Escribe a MagicHandy: el chat puede iniciar, ajustar y detener el movimiento | Fale com a MagicHandy: o chat pode iniciar, ajustar e parar o movimento | 给 MagicHandy 发消息——聊天可以启动、调整和停止运动 | MagicHandy にメッセージ — チャットでモーションの開始・調整・停止ができます |
| Send | Enviar | Enviar | 发送 | 送信 |
| Ctrl+Enter to send | Ctrl+Intro para enviar | Ctrl+Enter para enviar | 按 Ctrl+Enter 发送 | Ctrl+Enter で送信 |
| Jump to latest | Ir al más reciente | Ir para o mais recente | 跳到最新 | 最新へ移動 |
| You | Tú | Você | 你 | あなた |
| Streaming | Transmitiendo | Transmitindo | 正在生成 | 生成中 |
| Needs attention | Requiere atención | Requer atenção | 需要注意 | 要確認 |
| Failed | Falló | Falhou | 失败 | 失敗 |
| Repaired model JSON | JSON del modelo corregido | JSON do modelo corrigido | 已修复模型 JSON | モデル JSON を修復しました |
| Malformed model JSON | JSON del modelo mal formado | JSON do modelo malformado | 模型 JSON 格式错误 | モデル JSON の形式が不正です |
| Malformed model response. | Respuesta del modelo mal formada. | Resposta do modelo malformada. | 模型响应格式错误。 | モデルの応答形式が不正です。 |
| Stopping motion. | Deteniendo el movimiento. | Parando o movimento. | 正在停止运动。 | モーションを停止します。 |
| Start hands-free voice | Iniciar voz manos libres | Iniciar voz com mãos livres | 启动免提语音 | ハンズフリー音声を開始 |
| Stop and transcribe | Detener y transcribir | Parar e transcrever | 停止并转写 | 停止して文字起こし |
| Hold to talk | Mantener pulsado para hablar | Mantenha pressionado para falar | 按住说话 | 長押しして話す |
| Hands-free | Manos libres | Mãos livres | 免提 | ハンズフリー |
| Voice mode | Modo de voz | Modo de voz | 语音模式 | 音声モード |
| Voice input | Entrada de voz | Entrada de voz | 语音输入 | 音声入力 |
| Default microphone | Micrófono predeterminado | Microfone padrão | 默认麦克风 | 既定のマイク |
| Release microphone | Liberar micrófono | Liberar microfone | 释放麦克风 | マイクを解放 |
| Open voice input menu (aria) | Abrir menú de entrada de voz | Abrir menu de entrada de voz | 打开语音输入菜单 | 音声入力メニューを開く |
| Close voice input menu (aria) | Cerrar menú de entrada de voz | Fechar menu de entrada de voz | 关闭语音输入菜单 | 音声入力メニューを閉じる |
| Starting microphone | Iniciando micrófono | Iniciando microfone | 正在启动麦克风 | マイクを起動中 |
| Cancel microphone startup | Cancelar inicio del micrófono | Cancelar inicialização do microfone | 取消启动麦克风 | マイクの起動をキャンセル |
| Listening | Escuchando | Ouvindo | 正在聆听 | 聞き取り中 |
| Transcribing | Transcribiendo | Transcrevendo | 正在转写 | 文字起こし中 |
| Voice input active (aria) | Entrada de voz activa | Entrada de voz ativa | 语音输入已启用 | 音声入力中 |
| No speech was recognized. | No se reconoció ninguna voz. | Nenhuma fala foi reconhecida. | 未识别到语音。 | 音声を認識できませんでした。 |
| Transcription upload timed out. | Se agotó el tiempo para subir la transcripción. | O envio da transcrição expirou. | 转写上传超时。 | 文字起こしのアップロードがタイムアウトしました。 |
| The selected microphone became unavailable. | El micrófono seleccionado dejó de estar disponible. | O microfone selecionado ficou indisponível. | 所选麦克风不可用。 | 選択したマイクが利用できなくなりました。 |
| Chat canceled by Emergency Stop. | Chat cancelado por la parada de emergencia. | Chat cancelado pela parada de emergência. | 聊天已被紧急停止取消。 | 緊急停止によりチャットをキャンセルしました。 |
| Emergency Stop canceled this reply's motion and speech. | La parada de emergencia canceló el movimiento y la voz de esta respuesta. | A parada de emergência cancelou o movimento e a fala desta resposta. | 紧急停止已取消此回复的运动与语音。 | 緊急停止によりこの返信のモーションと音声をキャンセルしました。 |
| Device Stop could not be confirmed | No se pudo confirmar la detención del dispositivo | Não foi possível confirmar a parada do dispositivo | 无法确认设备停止 | デバイスの停止を確認できませんでした |
| Motion command failed | Falló el comando de movimiento | O comando de movimento falhou | 运动指令失败 | モーションコマンドが失敗しました |
| Chat history is unavailable; the reply was not applied. | El historial del chat no está disponible; la respuesta no se aplicó. | O histórico do chat está indisponível; a resposta não foi aplicada. | 聊天历史不可用；该回复未生效。 | チャット履歴が利用できないため、返信は適用されませんでした。 |

## Pattern Library — Import

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Import motion content | Importar contenido de movimiento | Importar conteúdo de movimento | 导入运动内容 | モーション内容をインポート |
| Choose file | Elegir archivo | Escolher arquivo | 选择文件 | ファイルを選択 |
| Reading file | Leyendo archivo | Lendo arquivo | 正在读取文件 | ファイルを読み取り中 |
| Checking the selected motion file. | Comprobando el archivo de movimiento seleccionado. | Verificando o arquivo de movimento selecionado. | 正在检查所选运动文件。 | 選択したモーションファイルを確認しています。 |
| Timeline view | Vista de línea de tiempo | Visualização da linha do tempo | 时间轴视图 | タイムライン表示 |
| Earlier | Anterior | Anterior | 更早 | 前へ |
| Later | Posterior | Posterior | 更晚 | 次へ |
| Zoom in | Acercar | Ampliar | 放大 | 拡大 |
| Zoom out | Alejar | Reduzir | 缩小 | 縮小 |
| Fit selection | Ajustar a la selección | Ajustar à seleção | 适合选择范围 | 選択範囲に合わせる |
| Fit all | Mostrar todo | Mostrar tudo | 显示全部 | 全体を表示 |
| Visible timeline range (aria) | Rango visible de la línea de tiempo | Intervalo visível da linha do tempo | 可见时间轴范围 | 表示中のタイムライン範囲 |
| Timeline viewport (aria) | Ventana de la línea de tiempo | Janela da linha do tempo | 时间轴视口 | タイムライン表示範囲 |
| Scroll to zoom at cursor; Shift-scroll to pan (tooltip) | Desplázate para ampliar en el cursor; Mayús-desplazamiento para mover | Role para ampliar no cursor; Shift-rolagem para mover | 滚动以光标为中心缩放；按住 Shift 滚动以平移 | スクロールでカーソル位置を拡大縮小、Shift+スクロールで移動 |
| Drag to move the visible timeline range (tooltip) | Arrastra para mover el rango visible de la línea de tiempo | Arraste para mover o intervalo visível da linha do tempo | 拖动以移动可见时间轴范围 | ドラッグして表示中のタイムライン範囲を移動 |
| Viewing {start}-{end} at {zoom} | Vista {start}-{end} con zoom {zoom} | Exibindo {start}-{end} em {zoom} | 以 {zoom} 查看 {start}-{end} | {start}-{end}を{zoom}で表示 |
| Funscript timeline editor, {total} total, viewing {start} to {end}, selection {selectionStart} to {selectionEnd}, {duration} selected (aria) | Editor de línea de tiempo de Funscript, {total} en total, vista de {start} a {end}, selección de {selectionStart} a {selectionEnd}, {duration} seleccionados | Editor da linha do tempo do Funscript, {total} no total, exibindo de {start} a {end}, seleção de {selectionStart} a {selectionEnd}, {duration} selecionados | Funscript 时间轴编辑器，总计 {total}，正在查看 {start} 到 {end}，选择范围 {selectionStart} 到 {selectionEnd}，已选择 {duration} | Funscriptタイムラインエディター、全体{total}、{start}から{end}を表示、選択範囲{selectionStart}から{selectionEnd}、選択時間{duration} |
| Trim start (aria) | Inicio del recorte | Início do recorte | 裁剪起点 | トリミング開始 |
| Trim end (aria) | Fin del recorte | Fim do recorte | 裁剪终点 | トリミング終了 |
| Current trim selection length (aria) | Duración de la selección de recorte actual | Duração da seleção de recorte atual | 当前裁剪选择长度 | 現在のトリミング選択時間 |
| Selection length {duration} | Duración de la selección {duration} | Duração da seleção {duration} | 选择长度 {duration} | 選択時間 {duration} |
| Name must be 80 characters or fewer. | El nombre debe tener 80 caracteres o menos. | O nome deve ter no máximo 80 caracteres. | 名称不得超过 80 个字符。 | 名前は80文字以内にしてください。 |
| Name cannot contain path separators (/ or \). | El nombre no puede contener separadores de ruta (/ o \). | O nome não pode conter separadores de caminho (/ ou \). | 名称不能包含路径分隔符（/ 或 \）。 | 名前にパス区切り文字（/ または \）は使用できません。 |
| {file} exceeds the 8 MiB import limit. | {file} supera el límite de importación de 8 MiB. | {file} excede o limite de importação de 8 MiB. | {file} 超过 8 MiB 导入限制。 | {file} は 8 MiB のインポート上限を超えています。 |
| {file} could not be read. | No se pudo leer {file}. | Não foi possível ler {file}. | 无法读取 {file}。 | {file} を読み取れませんでした。 |
| {file} uses an unknown motion content schema. | {file} usa un esquema de contenido de movimiento desconocido. | {file} usa um esquema de conteúdo de movimento desconhecido. | {file} 使用未知的运动内容架构。 | {file} は不明なモーション内容スキーマを使用しています。 |
| {file} must contain 2 to 20480 source actions. | {file} debe contener entre 2 y 20480 acciones de origen. | {file} deve conter de 2 a 20480 ações de origem. | {file} 必须包含 2 到 20480 个源动作。 | {file} には2～20480個のソースアクションが必要です。 |
| {file} has an invalid funscript version. | {file} tiene una versión de funscript no válida. | {file} tem uma versão de funscript inválida. | {file} 的 funscript 版本无效。 | {file} の funscript バージョンが無効です。 |
| {file} has an invalid inverted flag. | {file} tiene un indicador de inversión no válido. | {file} tem um sinalizador de inversão inválido. | {file} 的反转标志无效。 | {file} の反転フラグが無効です。 |
| {file} action {number} is not usable. | La acción {number} de {file} no se puede usar. | A ação {number} de {file} não pode ser usada. | {file} 的动作 {number} 不可用。 | {file} のアクション {number} は使用できません。 |
| {file} action {number} has an invalid time. | La acción {number} de {file} tiene un tiempo no válido. | A ação {number} de {file} tem um tempo inválido. | {file} 的动作 {number} 时间无效。 | {file} のアクション {number} の時刻が無効です。 |
| {file} action {number} position must be between 0 and 100. | La posición de la acción {number} de {file} debe estar entre 0 y 100. | A posição da ação {number} de {file} deve estar entre 0 e 100. | {file} 的动作 {number} 位置必须在 0 到 100 之间。 | {file} のアクション {number} の位置は0～100である必要があります。 |
| Programs preserve the selected knots and duration, play once, and use a 500 ms minimum playback period. | Los programas conservan los nodos y la duración seleccionados, se reproducen una vez y usan un periodo mínimo de reproducción de 500 ms. | Os programas preservam os nós e a duração selecionados, são reproduzidos uma vez e usam um período mínimo de reprodução de 500 ms. | 程序保留所选节点和时长，仅播放一次，并使用 500 毫秒的最短播放周期。 | プログラムは選択したノットと時間を保持して1回再生し、最短再生時間は500ミリ秒です。 |
| Loop patterns repeat. Active timing remains as selected; cycles shorter than 6.6 seconds are safety-stretched to 6.6 seconds. Qualifying stationary pauses over 5 seconds collapse, positions expand to the full relative span, and the loop closes. | Los patrones en bucle se repiten. El tiempo activo se conserva según la selección; los ciclos de menos de 6,6 segundos se alargan por seguridad hasta 6,6 segundos. Las pausas estacionarias válidas de más de 5 segundos se reducen, las posiciones se amplían al rango relativo completo y el bucle se cierra. | Os padrões em loop se repetem. O tempo ativo permanece como selecionado; ciclos menores que 6,6 segundos são estendidos por segurança até 6,6 segundos. Pausas estacionárias qualificadas acima de 5 segundos são reduzidas, as posições são ampliadas para o intervalo relativo completo e o loop é fechado. | 循环模式会重复。活动时序保留所选设置；短于 6.6 秒的周期会出于安全原因拉伸到 6.6 秒。符合条件的 5 秒以上静止停顿会被压缩，位置扩展到完整相对范围，并闭合循环。 | ループパターンは反復します。動作中のタイミングは選択どおり保持され、6.6秒未満のサイクルは安全のため6.6秒まで引き伸ばされます。条件を満たす5秒超の静止区間は短縮され、位置は相対範囲全体に拡張され、ループが閉じられます。 |
| This loop has {count} essential reversal knots; trim to a simpler section with 255 or fewer. | Este bucle tiene {count} nodos de inversión esenciales; recorta una sección más simple con 255 o menos. | Este loop tem {count} nós de reversão essenciais; corte uma seção mais simples com 255 ou menos. | 此循环包含 {count} 个必要的反转节点；请裁剪为不超过 255 个节点的更简单片段。 | このループには必須の反転ノットが{count}個あります。255個以下のより単純な範囲にトリミングしてください。 |

## Settings — Shell

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Settings | Ajustes | Configurações | 设置 | 設定 |
| Close | Cerrar | Fechar | 关闭 | 閉じる |
| Device | Dispositivo | Dispositivo | 设备 | デバイス |
| Model | Modelo | Modelo | 模型 | モデル |
| Prompts & Memory | Prompts y memoria | Prompts e memória | 提示词与记忆 | プロンプトとメモリ |
| Diagnostics | Diagnóstico | Diagnóstico | 诊断 | 診断 |
| Save settings | Guardar ajustes | Salvar configurações | 保存设置 | 設定を保存 |
| Not loaded | Sin cargar | Não carregado | 未加载 | 未読み込み |
| Saving | Guardando | Salvando | 正在保存 | 保存中 |
| Saved | Guardado | Salvo | 已保存 | 保存しました |
| Save failed | Error al guardar | Falha ao salvar | 保存失败 | 保存に失敗しました |
| Settings saved. | Ajustes guardados. | Configurações salvas. | 设置已保存。 | 設定を保存しました。 |

## Settings — Device Connection

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Device Connection | Conexión del dispositivo | Conexão do dispositivo | 设备连接 | デバイス接続 |
| HSP dispatch owner | Origen de envío HSP | Origem de envio HSP | HSP 指令来源 | HSP 送信元 |
| Firmware / API requirement | Requisito de firmware / API | Requisito de firmware / API | 固件 / API 要求 | ファームウェア / API 要件 |
| API application ID source | Origen del ID de aplicación de la API | Origem do ID de aplicativo da API | API 应用 ID 来源 | API アプリケーション ID の取得元 |
| Developer application ID | ID de aplicación de desarrollador | ID de aplicativo de desenvolvedor | 开发者应用 ID | 開発者アプリケーション ID |
| Handy connection key | Clave de conexión de Handy | Chave de conexão do Handy | Handy 连接密钥 | Handy 接続キー |
| Clear connection key | Borrar clave de conexión | Limpar chave de conexão | 清除连接密钥 | 接続キーを消去 |
| Configured | Configurada | Configurada | 已配置 | 設定済み |
| Bluetooth disconnected | Bluetooth desconectado | Bluetooth desconectado | 蓝牙已断开 | Bluetooth 未接続 |
| Connect | Conectar | Conectar | 连接 | 接続 |
| Disconnect | Desconectar | Desconectar | 断开 | 切断 |
| Browser | Navegador | Navegador | 浏览器 | ブラウザ |
| Bridge | Puente | Ponte | 桥接 | ブリッジ |
| Check connection | Comprobar conexión | Verificar conexão | 检查连接 | 接続を確認 |
| Not checked | Sin comprobar | Não verificado | 未检查 | 未確認 |
| Connected: HSP ready | Conectado: HSP listo | Conectado: HSP pronto | 已连接：HSP 就绪 | 接続済み：HSP 準備完了 |
| HSP unavailable | HSP no disponible | HSP indisponível | HSP 不可用 | HSP 利用不可 |
| Server port | Puerto del servidor | Porta do servidor | 服务器端口 | サーバーポート |

## Settings — Local LLM (Model)

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Local LLM | LLM local | LLM local | 本地 LLM | ローカル LLM |
| Provider | Proveedor | Provedor | 提供方 | プロバイダー |
| llama.cpp mode | Modo llama.cpp | Modo llama.cpp | llama.cpp 模式 | llama.cpp モード |
| Model | Modelo | Modelo | 模型 | モデル |
| llama.cpp URL | URL de llama.cpp | URL do llama.cpp | llama.cpp 地址 | llama.cpp の URL |
| Managed llama.cpp runtime | Entorno llama.cpp gestionado | Runtime llama.cpp gerenciado | 托管 llama.cpp 运行时 | 管理対象 llama.cpp ランタイム |
| Build backend | Backend de compilación | Backend de compilação | 构建后端 | ビルドバックエンド |
| Auto-detect | Detección automática | Detectar automaticamente | 自动检测 | 自動検出 |
| Build runtime | Compilar entorno | Compilar runtime | 构建运行时 | ランタイムをビルド |
| Build / switch runtime | Compilar / cambiar entorno | Compilar / trocar runtime | 构建 / 切换运行时 | ランタイムをビルド / 切替 |
| Cancel build | Cancelar compilación | Cancelar compilação | 取消构建 | ビルドをキャンセル |
| Built from pinned source | Compilado desde código fijado | Compilado de fonte fixada | 从固定源码构建 | 固定ソースからビルド |
| Ollama URL | URL de Ollama | URL do Ollama | Ollama 地址 | Ollama の URL |
| Managed models | Modelos gestionados | Modelos gerenciados | 托管模型 | 管理対象モデル |
| Refresh model list | Actualizar lista de modelos | Atualizar lista de modelos | 刷新模型列表 | モデル一覧を更新 |
| Import GGUF | Importar GGUF | Importar GGUF | 导入 GGUF | GGUF をインポート |
| Import from Ollama | Importar desde Ollama | Importar do Ollama | 从 Ollama 导入 | Ollama からインポート |
| Ollama models path | Ruta de modelos de Ollama | Caminho dos modelos do Ollama | Ollama 模型路径 | Ollama モデルのパス |
| Scan library | Explorar biblioteca | Verificar biblioteca | 扫描模型库 | ライブラリをスキャン |
| Filter models | Filtrar modelos | Filtrar modelos | 筛选模型 | モデルを絞り込む |
| Import copy | Importar copia | Importar cópia | 导入副本 | コピーをインポート |
| Remove copy | Eliminar copia | Remover cópia | 删除副本 | コピーを削除 |
| No managed models. | No hay modelos gestionados. | Nenhum modelo gerenciado. | 没有托管模型。 | 管理対象モデルはありません。 |
| Save settings before runtime actions. | Guarda los ajustes antes de controlar el proceso. | Salve as configurações antes de controlar o processo. | 执行运行时操作前请先保存设置。 | ランタイム操作の前に設定を保存してください。 |
| Timeout ms | Tiempo de espera (ms) | Tempo limite (ms) | 超时（毫秒） | タイムアウト（ミリ秒） |
| Check | Comprobar | Verificar | 检查 | 確認 |
| Load | Cargar | Carregar | 加载 | 読み込み |
| Unload | Descargar | Descarregar | 卸载 | 解放 |
| Ready | Listo | Pronto | 就绪 | 準備完了 |
| Loaded, not ready | Cargado, no listo | Carregado, não pronto | 已加载，未就绪 | 読み込み済み・未準備 |
| Not ready | No listo | Não pronto | 未就绪 | 未準備 |

## Settings — Prompts & Memory

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Active prompt set | Conjunto de prompts activo | Conjunto de prompts ativo | 当前提示词集 | 有効なプロンプトセット |
| saved with Save settings | se guarda con «Guardar ajustes» | salvo com "Salvar configurações" | 通过“保存设置”保存 | 「設定を保存」で保存されます |
| Prompt set editor | Editor de conjuntos de prompts | Editor de conjuntos de prompts | 提示词集编辑器 | プロンプトセット編集 |
| Edit set | Editar conjunto | Editar conjunto | 编辑集 | セットを編集 |
| Name | Nombre | Nome | 名称 | 名前 |
| Built-in (read-only) | Integrado (solo lectura) | Integrado (somente leitura) | 内置（只读） | 組み込み（読み取り専用） |
| Behavior instructions | Instrucciones de comportamiento | Instruções de comportamento | 行为说明 | 動作の指示 |
| the motion JSON contract is enforced by code and cannot be edited | el contrato JSON de movimiento lo aplica el código y no se puede editar | o contrato JSON de movimento é imposto pelo código e não pode ser editado | 运动 JSON 契约由代码强制执行，无法编辑 | モーション JSON の規約はコードで強制され、編集できません |
| New set | Nuevo conjunto | Novo conjunto | 新建集 | 新規セット |
| Duplicate as new | Duplicar como nuevo | Duplicar como novo | 复制为新集 | 複製して新規作成 |
| Save set | Guardar conjunto | Salvar conjunto | 保存集 | セットを保存 |
| Delete set | Eliminar conjunto | Excluir conjunto | 删除集 | セットを削除 |
| New set — Save set to keep it. | Nuevo conjunto: pulsa «Guardar conjunto» para conservarlo. | Novo conjunto — clique em "Salvar conjunto" para mantê-lo. | 新集——点击“保存集”以保留。 | 新規セット — 「セットを保存」で保存されます。 |
| Created. | Creado. | Criado. | 已创建。 | 作成しました。 |
| Saved. | Guardado. | Salvo. | 已保存。 | 保存しました。 |
| Deleted. | Eliminado. | Excluído. | 已删除。 | 削除しました。 |
| Confirm: delete this set | Confirmar: eliminar este conjunto | Confirmar: excluir este conjunto | 确认：删除此集 | 確認：このセットを削除 |
| Long-term memory | Memoria a largo plazo | Memória de longo prazo | 长期记忆 | 長期メモリ |
| Include saved memories in chat | Incluir memorias guardadas en el chat | Incluir memórias salvas no chat | 在聊天中包含已保存的记忆 | 保存済みメモリをチャットに含める |
| No memories saved yet. | Aún no hay memorias guardadas. | Nenhuma memória salva ainda. | 尚无已保存的记忆。 | 保存済みメモリはまだありません。 |
| New memory | Nueva memoria | Nova memória | 新记忆 | 新規メモリ |
| A short fact the assistant should remember | Un dato breve que el asistente debe recordar | Um fato curto que o assistente deve lembrar | 助手应记住的简短信息 | アシスタントに覚えさせたい短い情報 |
| Add memory | Añadir memoria | Adicionar memória | 添加记忆 | メモリを追加 |
| Remove | Quitar | Remover | 移除 | 削除 |
| Clear all | Borrar todo | Limpar tudo | 全部清除 | すべて消去 |
| Enter the memory text first. | Escribe primero el texto de la memoria. | Digite o texto da memória primeiro. | 请先输入记忆内容。 | 先にメモリの内容を入力してください。 |
| Added. | Añadida. | Adicionada. | 已添加。 | 追加しました。 |
| Removed. | Quitada. | Removida. | 已移除。 | 削除しました。 |
| Memory on. | Memoria activada. | Memória ativada. | 记忆已开启。 | メモリをオンにしました。 |
| Memory off — chat runs without it. | Memoria desactivada: el chat funciona sin ella. | Memória desativada — o chat funciona sem ela. | 记忆已关闭——聊天将不使用它。 | メモリをオフにしました — チャットは使用せず動作します。 |
| Confirm: delete every memory | Confirmar: eliminar todas las memorias | Confirmar: excluir todas as memórias | 确认：删除所有记忆 | 確認：すべてのメモリを削除 |
| All memories removed. | Todas las memorias eliminadas. | Todas as memórias removidas. | 已删除所有记忆。 | すべてのメモリを削除しました。 |

## Settings — Chat Voice, Persona, and User Anatomy

MagicHandy exposes these current strings in Settings > Prompts & memory. User
anatomy describes what the device is being used on and remains separate from
the partner persona. The compact Chat mood value is backend-reported state, not
frontend sentiment analysis.

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Chat voice | Voz del chat | Voz do chat | 聊天语气 | チャットの話し方 |
| how sexual the model's replies may be | grado de contenido sexual permitido en las respuestas del modelo | nível de conteúdo sexual permitido nas respostas do modelo | 模型回复可包含的性内容程度 | モデルの返答で許可する性的表現の度合い |
| Utility (neutral assistant) | Utilitaria (asistente neutral) | Utilitária (assistente neutro) | 实用（中性助手） | 実用（中立的なアシスタント） |
| Warm (flirtatious, never explicit) | Cálida (coqueta, nunca explícita) | Calorosa (paqueradora, nunca explícita) | 温暖（调情，但不露骨） | ウォーム（親密だが露骨ではない） |
| Intimate (sensual partner) | Íntima (pareja sensual) | Íntima (parceiro sensual) | 亲密（感性的伴侣） | インティメート（官能的なパートナー） |
| Explicit (direct sexual language) | Explícita (lenguaje sexual directo) | Explícita (linguagem sexual direta) | 露骨（直接的性语言） | 露骨（直接的な性的表現） |
| Utility keeps the neutral assistant register. | Utilitaria mantiene el registro de asistente neutral. | Utilitária mantém o registro de assistente neutro. | 实用模式保持中性助手语气。 | 実用は中立的なアシスタント口調を保ちます。 |
| Warm is flirtatious but never explicit. | Cálida es coqueta, pero nunca explícita. | Calorosa é paqueradora, mas nunca explícita. | 温暖模式可以调情，但绝不露骨。 | ウォームは親密ですが、露骨にはなりません。 |
| Intimate speaks as a partner with sensual language. | Íntima habla como una pareja con lenguaje sensual. | Íntima fala como um parceiro com linguagem sensual. | 亲密模式以伴侣身份使用感性语言。 | インティメートは官能的な言葉でパートナーとして話します。 |
| Explicit permits direct sexual language like the legacy app. | Explícita permite lenguaje sexual directo como la aplicación anterior. | Explícita permite linguagem sexual direta como o aplicativo legado. | 露骨模式允许像旧版应用一样使用直接的性语言。 | 露骨は旧アプリと同様に直接的な性的表現を許可します。 |
| Voice changes wording only; motion limits, capability gates, and Stop are identical at every level. | La voz solo cambia la redacción; los límites de movimiento, los permisos y Stop son idénticos en todos los niveles. | A voz altera apenas a redação; limites de movimento, permissões e Stop são idênticos em todos os níveis. | 语气只改变措辞；所有级别的运动限制、能力权限和停止功能完全相同。 | 話し方は文面だけを変えます。モーション制限、権限、停止はすべてのレベルで同一です。 |
| User anatomy | Anatomía del usuario | Anatomia do usuário | 用户身体部位 | ユーザーの身体部位 |
| separate from partner persona | separado de la persona de la pareja | separado da persona do parceiro | 与伴侣角色分开 | パートナーのペルソナとは別 |
| Penis | Pene | Pênis | 阴茎 | ペニス |
| Vagina / vulva | Vagina / vulva | Vagina / vulva | 阴道 / 外阴 | 膣 / 外陰部 |
| Custom wording | Texto personalizado | Texto personalizado | 自定义称呼 | カスタム表現 |
| Custom anatomy wording | Texto personalizado para la anatomía | Texto personalizado da anatomia | 自定义身体称呼 | カスタム身体表現 |
| Persona description | Descripción de la persona | Descrição da persona | 角色描述 | ペルソナの説明 |
| optional | opcional | opcional | 可选 | 任意 |
| Anatomy vocabulary and persona apply to interactive Warm, Intimate, and Explicit replies only. | El vocabulario anatómico y la persona solo se aplican a las respuestas interactivas Cálida, Íntima y Explícita. | O vocabulário anatômico e a persona se aplicam apenas às respostas interativas Calorosa, Íntima e Explícita. | 身体词汇和角色仅适用于交互式温暖、亲密和露骨回复。 | 身体表現とペルソナは、対話形式のウォーム、インティメート、露骨の返答にのみ適用されます。 |
| They are bounded prompt context and cannot change motion permissions or limits. | Son contexto acotado del prompt y no pueden cambiar los permisos ni los límites de movimiento. | São contexto limitado do prompt e não podem alterar permissões nem limites de movimento. | 它们是有界的提示上下文，无法更改运动权限或限制。 | これらは長さを制限したプロンプト文脈であり、モーションの権限や制限を変更できません。 |
| Mood | Estado | Humor | 状态 | ムード |
| Assistant mood: {mood} | Estado del asistente: {mood} | Humor do assistente: {mood} | 助手状态：{mood} | アシスタントのムード: {mood} |

## Settings — Diagnostics

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Diagnostics verbosity | Nivel de detalle del diagnóstico | Nível de detalhe do diagnóstico | 诊断详细程度 | 診断の詳細度 |
| Copy summary | Copiar resumen | Copiar resumo | 复制摘要 | 概要をコピー |
| Copied | Copiado | Copiado | 已复制 | コピーしました |
| Export trace | Exportar traza | Exportar rastreamento | 导出追踪 | トレースを書き出し |
| Engine | Motor | Motor | 引擎 | エンジン |
| Estimated position | Posición estimada | Posição estimada | 估算位置 | 推定位置 |
| Transport | Transporte | Transporte | 传输方式 | トランスポート |
| Playback | Reproducción | Reprodução | 播放 | 再生 |
| Last command | Último comando | Último comando | 上一条指令 | 直近のコマンド |
| Command latency | Latencia del comando | Latência do comando | 指令延迟 | コマンド遅延 |
| Last error | Último error | Último erro | 上一个错误 | 直近のエラー |
| Core | Núcleo | Núcleo | 核心 | コア |
| Connection key | Clave de conexión | Chave de conexão | 连接密钥 | 接続キー |
| Motion feature | Función de movimiento | Recurso de movimento | 运动功能 | モーション機能 |
| Version | Versión | Versão | 版本 | バージョン |
| Commit | Commit | Commit | 提交 | コミット |
| Uptime | Tiempo activo | Tempo ativo | 运行时长 | 稼働時間 |
| Health | Estado | Integridade | 健康状态 | ヘルス |
| None | Ninguno | Nenhum | 无 | なし |
| Reset | Restablecer | Redefinir | 重置 | リセット |
| Reset all settings | Restablecer todos los ajustes | Redefinir todas as configurações | 重置所有设置 | すべての設定をリセット |
| Restores every setting to factory defaults, including the connection key. Saved memories and prompt sets are not touched. | Restaura todos los ajustes a los valores de fábrica, incluida la clave de conexión. Las memorias y los conjuntos de prompts guardados no se modifican. | Restaura todas as configurações para o padrão de fábrica, incluindo a chave de conexão. As memórias e os conjuntos de prompts salvos não são afetados. | 将所有设置恢复为出厂默认值，包括连接密钥。已保存的记忆和提示词集不受影响。 | 接続キーを含むすべての設定を工場出荷時に戻します。保存済みのメモリとプロンプトセットは変更されません。 |
| Confirm: reset every setting | Confirmar: restablecer todos los ajustes | Confirmar: redefinir todas as configurações | 确认：重置所有设置 | 確認：すべての設定をリセット |
| Reset. Reloading… | Restablecido. Recargando… | Redefinido. Recarregando… | 已重置。正在重新加载… | リセットしました。再読み込み中… |

## LLM-Facing Wording (prompt sets & personas)

These are sent to the local model, not shown as ordinary UI. MagicHandy's
built-in prompt sets use a hybrid localization strategy: behavior text and
memory headers are localized, while the machine JSON contract appended by
`internal/chat/prompts.go` stays code-owned and English so JSON keys and enum
values remain stable. The current voice, anatomy, profile-data, mood-state, and
recent-line instructions are also code-owned English pending the native-speaker
pass recorded in `docs/chat-voice.md`; every built-in still explicitly sets the
`reply` language. See `docs/prompt-localization-strategy.md` for the rationale
and current composition order.

StrokeGPT-ReVibed's default `revibed` prompt was explicitly adult; future
MagicHandy prompt packs may be explicit and should be translated at the same
tone rather than sanitized.

**Default prompt set IDs** (`internal/chat/prompts.go`):

| lang | id | name |
| --- | --- | --- |
| en | `magichandy_motion_v1` | MagicHandy Motion (default) |
| es | `magichandy_motion_v1_es` | MagicHandy Motion (Spanish) |
| pt-BR | `magichandy_motion_v1_pt_br` | MagicHandy Motion (Portuguese, Brazil) |
| zh-Hans | `magichandy_motion_v1_zh_hans` | MagicHandy Motion (Simplified Chinese) |
| ja | `magichandy_motion_v1_ja` | MagicHandy Motion (Japanese) |

**Default prompt behavior text.** The app appends the English
`ContractInstructions` after each block; do not translate `reply`, JSON keys,
enum values, or pattern IDs.

English (`magichandy_motion_v1`):

```text
You are MagicHandy's local motion assistant. Be warm, concise, and
attentive to what the user asks for. Match the user's energy without
escalating beyond their requests.
Write the user-facing `reply` value in English. Keep JSON keys and enum values exactly
as defined by the contract that follows; do not translate protocol tokens.
```

Spanish (`magichandy_motion_v1_es`):

```text
Eres el asistente local de movimiento de MagicHandy. Sé cálido, conciso y
atento a lo que pide el usuario. Adáptate a su energía sin ir más allá de lo
que solicita.
Escribe el valor de `reply` dirigido al usuario en español. Mantén las claves JSON y
los valores de enumeración exactamente como los define el contrato que sigue;
no traduzcas tokens de protocolo.
```

Portuguese, Brazil (`magichandy_motion_v1_pt_br`):

```text
Você é o assistente local de movimento da MagicHandy. Seja acolhedor,
conciso e atento ao que o usuário pede. Acompanhe a energia do usuário sem ir
além do que ele solicita.
Escreva o valor de `reply` voltado ao usuário em português do Brasil. Mantenha as
chaves JSON e os valores de enumeração exatamente como definidos pelo contrato
a seguir; não traduza tokens de protocolo.
```

Simplified Chinese (`magichandy_motion_v1_zh_hans`):

```text
你是 MagicHandy 的本地运动助手。回应要温暖、简洁，并关注用户的需求。顺应用户的节奏，不要超出其要求的范围。
面向用户的 `reply` 值必须使用简体中文。JSON 键和枚举值必须严格保持后续契约定义的形式；不要翻译协议标记。
```

Japanese (`magichandy_motion_v1_ja`):

```text
あなたは MagicHandy のローカル・モーションアシスタントです。温かく簡潔に、ユーザーの求めに寄り添って応答してください。ユーザーの熱量に合わせ、要求を超えてエスカレートさせないでください。
ユーザー向けの `reply` 値は日本語で書いてください。JSON キーと列挙値は後続の契約で定義されたとおりに保ち、プロトコル用トークンを翻訳しないでください。
```

**Saved-memory headers** (`memoryInstructionForPrompt`):

| lang | text |
| --- | --- |
| en | Saved user memories (reference naturally when relevant; never recite the list): |
| es | Memorias guardadas del usuario (haz referencia a ellas con naturalidad cuando sean relevantes; nunca recites la lista): |
| pt-BR | Memórias salvas do usuário (use-as com naturalidade quando forem relevantes; nunca recite a lista): |
| zh-Hans | 已保存的用户记忆（相关时自然引用；不要逐条背诵列表）： |
| ja | 保存済みのユーザーメモリ（関連する場合だけ自然に参照し、一覧を読み上げないこと）: |

**Mood protocol values** (`new_mood`) stay exact English tokens in every
language, like motion enums: `Curious`, `Teasing`, `Playful`, `Loving`,
`Excited`, `Passionate`, `Seductive`, `Anticipatory`, `Breathless`, `Dominant`,
`Submissive`, `Vulnerable`, `Confident`, `Intimate`, `Needy`, `Overwhelmed`,
`Afterglow`. A future localized UI may translate the displayed label, but the
wire value and strict parser enum do not change.

| protocol / en display | es display | pt-BR display | zh-Hans display | ja display |
| --- | --- | --- | --- | --- |
| Curious | Curioso/a | Curioso/a | 好奇 | 好奇 |
| Teasing | Provocador/a | Provocante | 挑逗 | じらす |
| Playful | Juguetón/a | Brincalhão/ona | 调皮 | 遊び心 |
| Loving | Cariñoso/a | Carinhoso/a | 深情 | 愛情深い |
| Excited | Excitado/a | Excitado/a | 兴奋 | 興奮 |
| Passionate | Apasionado/a | Apaixonado/a | 热情 | 情熱的 |
| Seductive | Seductor/a | Sedutor/a | 诱惑 | 誘惑的 |
| Anticipatory | Expectante | Em expectativa | 期待 | 期待 |
| Breathless | Sin aliento | Sem fôlego | 喘息 | 息を切らした |
| Dominant | Dominante | Dominante | 支配 | 支配的 |
| Submissive | Sumiso/a | Submisso/a | 顺从 | 従順 |
| Vulnerable | Vulnerable | Vulnerável | 脆弱 | 無防備 |
| Confident | Seguro/a | Confiante | 自信 | 自信 |
| Intimate | Íntimo/a | Íntimo/a | 亲密 | 親密 |
| Needy | Necesitado/a | Carente | 渴求 | 求めている |
| Overwhelmed | Abrumado/a | Sobrecarregado/a | 不知所措 | 圧倒された |
| Afterglow | Plenitud | Relaxamento | 余韵 | 余韻 |

**Persona presets** (mild starter tier, mirroring StrokeGPT-ReVibed's
`DEFAULT_PERSONA_PROMPTS`). These are not the full legacy chat prompt; they are
only short persona descriptions:

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| An energetic and passionate girlfriend | Una novia enérgica y apasionada | Uma namorada enérgica e apaixonada | 一个精力充沛、热情的女友 | エネルギッシュで情熱的な彼女 |
| An energetic and passionate boyfriend | Un novio enérgico y apasionado | Um namorado enérgico e apaixonado | 一个精力充沛、热情的男友 | エネルギッシュで情熱的な彼氏 |
| An energetic and passionate partner | Una pareja enérgica y apasionada | Um parceiro enérgico e apaixonado | 一个精力充沛、热情的伴侣 | エネルギッシュで情熱的なパートナー |
| A warm, attentive companion | Un acompañante cálido y atento | Um companheiro caloroso e atencioso | 一个温暖、体贴的伴侣 | 温かく気配りのある相手 |
| A calm, reassuring presence | Una presencia tranquila y reconfortante | Uma presença calma e reconfortante | 一个沉稳、令人安心的存在 | 落ち着いて安心感のある存在 |

For full legacy prompt wording, including direct erotic-language requirements,
anatomy vocabulary, mode prompts, and memory/profile instructions, see
`docs/stgpt-rv-prompt-inventory.md`.

## Localization Guidance

- **Placeholders and units stay put.** `%`, `ms`, `MagicHandy`, `HSP`,
  `llama.cpp`, `Ollama`, `GGUF`, `Bluetooth`, `Handy`, and `Freestyle` (as a
  feature name) are not translated; only surrounding words are.
- **Keyboard hints** map to local conventions: `Ctrl+Enter` stays literal;
  `Esc` stays `Esc`.
- **Ellipsis** in progress strings ("Applying…", "Reloading…") uses the single
  `…` character in every language.
- **Tone:** functional strings are neutral/formal. Prompt/persona translations
  keep the source tone, including explicit adult language when present.
- **Prompt protocol:** prompt prose should be localized, but JSON keys and enum
  values stay in English per `docs/prompt-localization-strategy.md`.
- **No RTL languages** are in scope, so no layout mirroring is required.
- **Number/percent formatting** is unchanged (the UI shows `50%` etc.
  numerically); only labels are translated.
- When wiring an i18n framework later, key each row by a stable id (e.g.
  `bar.stop_everything`) and generate catalogs from this table; keep this doc
  as the human-readable source.
