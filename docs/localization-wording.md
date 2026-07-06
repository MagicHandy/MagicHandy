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
| biases Freestyle pacing | ajusta el ritmo del modo libre | ajusta o ritmo do modo livre | 影响自由模式的节奏 | フリースタイルのペースに影響 |
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
| llama-server path | Ruta de llama-server | Caminho do llama-server | llama-server 路径 | llama-server のパス |
| GGUF model path | Ruta del modelo GGUF | Caminho do modelo GGUF | GGUF 模型路径 | GGUF モデルのパス |
| Ollama URL | URL de Ollama | URL do Ollama | Ollama 地址 | Ollama の URL |
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

## Settings — Prompt Anatomy (Legacy Parity)

StrokeGPT-ReVibed exposed these strings in Settings > Persona. MagicHandy does
not yet have this exact surface, but future prompt/anatomy localization should
preserve the distinction between the user's anatomy and the persona's gender.

| en | es | pt-BR | zh-Hans | ja |
| --- | --- | --- | --- | --- |
| Prompt Anatomy | Anatomia del prompt | Anatomia do prompt | 提示词解剖设置 | プロンプトの身体設定 |
| User anatomy for prompts | Anatomia del usuario para prompts | Anatomia do usuário para prompts | 提示词中的用户身体 | プロンプト用のユーザー身体 |
| Penis | Pene | Penis | 阴茎 | ペニス |
| Vagina | Vagina | Vagina | 阴道 | 膣 |
| Custom | Personalizado | Personalizado | 自定义 | カスタム |
| Custom anatomy wording | Texto personalizado para la anatomía | Texto personalizado da anatomia | 自定义身体称呼 | カスタム身体表現 |
| Save Anatomy | Guardar anatomía | Salvar anatomia | 保存身体设置 | 身体設定を保存 |
| This tells the prompt what the device is being used on, separate from persona gender. | Esto le dice al prompt en qué anatomía se usa el dispositivo, separado del género de la persona. | Isso informa ao prompt em qual anatomia o dispositivo está sendo usado, separado do gênero da persona. | 这会告诉提示词设备正用于哪种身体部位，并与角色性别分开处理。 | これは、ペルソナの性別とは別に、デバイスがどの身体に使われているかをプロンプトへ伝えます。 |

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
built-in prompt sets use a hybrid localization strategy: behavior/persona prose
and memory headers are localized, but the machine JSON contract appended by
`internal/chat/prompts.go` stays code-owned and English so JSON keys and enum
values remain stable. See `docs/prompt-localization-strategy.md` for the
rationale.

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
