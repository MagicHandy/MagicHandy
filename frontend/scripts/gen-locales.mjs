import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const en = JSON.parse(
  fs.readFileSync(path.join(__dirname, "../src/i18n/locales/en.json"), "utf8"),
);

const pt = JSON.parse(JSON.stringify(en));
const fr = JSON.parse(JSON.stringify(en));
const ru = JSON.parse(JSON.stringify(en));

const ptMap = {
  "Loading…": "Carregando…",
  "Error": "Erro",
  "Save": "Salvar",
  "Delete": "Excluir",
  "Cancel": "Cancelar",
  "Close": "Fechar",
  "Send": "Enviar",
  "Stop": "Parar",
  "Confirm": "Confirmar",
  "Apply": "Aplicar",
  "Refresh": "Atualizar",
  "Export": "Exportar",
  "Import": "Importar",
  "Edit": "Editar",
  "Activate": "Ativar",
  "active": "ativa",
  "Yes": "Sim",
  "No": "Não",
  "None": "Nenhum",
  "Any": "Qualquer",
  "All": "Todos",
  "Search": "Buscar",
  "Use": "Usar",
  "Scan": "Scan",
  "Connect": "Conectar",
  "Connected": "Conectado",
  "Offline": "Offline",
  "Playing": "Tocando",
  "Paused": "Pausado",
  "Stopped": "Parado",
  "Remove": "Remover",
  "Add": "Adicionar",
  "New": "Novo",
  "Back": "Voltar",
  "Next": "Próximo",
  "Previous": "Anterior",
  "Download": "Baixar",
  "Upload": "Enviar",
  "Test": "Testar",
  "Play": "Tocar",
  "Pause": "Pausar",
  "Resume": "Retomar",
  "Skip": "Pular",
  "Clear": "Limpar",
  "Reset": "Resetar",
  "min": "mín",
  "max": "máx",
  "seconds": "segundos",
  "minutes": "minutos",
  "items": "itens",
  "blocks": "blocos",
  "block": "bloco",
  "queue": "fila",
  "Name": "Nome",
  "Description": "Descrição",
  "Notes": "Notas",
  "Rating": "Rating",
  "Mode": "Modo",
  "Status": "Status",
  "Duration": "Duração",
  "Intensity": "Intensidade",
  "Speed": "Velocidade",
  "Zone": "Zona",
  "Rhythm": "Ritmo",
  "Category": "Categoria",
  "Tags": "Tags",
  "Favorite": "Favorito",
  "Favorites": "Favoritos",
  "URL": "URL",
  "Model": "Modelo",
  "Provider": "Provedor",
  "Language": "Idioma",
  "Enabled": "Ativado",
  "Disabled": "Desativado",
  "On": "Ligado",
  "Off": "Desligado",
  "Slow": "Lento",
  "Medium": "Médio",
  "Fast": "Rápido",
  "Very fast": "Muito rápido",
  "Low": "Baixo",
  "High": "Alto",
  "Light": "Leve",
  "Intense": "Intenso",
  "Saved": "Salvo",
  "Deleted": "Excluído",
  "Exported": "Exportado",
  "Settings saved": "Configurações salvas",
  "JSON applied — click Save": "JSON aplicado — clique em Salvar",
  "Invalid JSON.": "JSON inválido.",
  "Invalid JSON — fix before saving": "JSON inválido — corrija antes de salvar",
  "Emergency stop": "Parada de emergência",
  "Clear stop": "Limpar parada",
  "Stop word triggered": "Parada por stop word",
  "API unavailable — start the backend": "API indisponível — inicie o backend",
  "Waiting for API…": "Aguardando API…",
  "UI version": "Versão da UI",
  "…": "…",
  "Choose your language": "Escolha seu idioma",
  "You can change this anytime in Settings → Language.": "Você pode mudar isso a qualquer momento em Configuração → Idioma.",
  "Select language": "Selecionar idioma",
  "Continue": "Continuar",
  "Main navigation": "Navegação principal",
  "Session": "Sessão",
  "Library": "Biblioteca",
  "System": "Sistema",
  "Control": "Controle",
  "Hands-free": "Hands-free",
  "Mouse": "Mouse",
  "Player": "Reprodutor",
  "Settings": "Configuração",
  "Playing: {{name}}": "Tocando: {{name}}",
  "{{count}} item(s) in queue": "{{count}} item(ns) na fila",
  "API offline": "API offline",
  "Start the backend:": "Inicie o backend:",
  "Reconnecting to Intiface…": "Reconectando ao Intiface…",
  "Ollama connected": "Ollama conectado",
  "Buffer": "Buffer",
  "Queue": "Fila",
  "Intens.": "Intens.",
  "locked": "travada",
  "advance phase": "trocar fase",
  "autospeak": "autospeak",
  "autospeak ·": "autospeak ·",
  "Test connection": "Testar conexão",
  "Local LLM": "LLM local",
  "Configuration": "Configuração",
  "Search…": "Buscar…",
  "Search configuration": "Buscar configuração",
  "No sections found.": "Nenhuma seção encontrada.",
  "Main": "Principal",
  "Data": "Dados",
  "Interface language": "Idioma da interface",
  "Changes apply immediately and are saved to your preferences.": "Mudanças aplicam na hora e são salvas nas preferências.",
  "Interface language and locale": "Idioma e localidade da interface",
  "Personas": "Personas",
  "Name, prompt, and AI profile": "Nome, prompt e perfil da IA",
  "Session & AI": "Sessão & IA",
  "Autospeak, scene, and persona rhythm": "Autospeak, cena e ritmo da persona",
  "Motion & queue": "Movimento & fila",
  "Position, buffer, safety, and planner": "Posição, buffer, segurança e planner",
  "Connections": "Conexões",
  "Ollama, Handy, Intiface, and sync": "Ollama, Handy, Intiface e sync",
  "Voice": "Voz",
  "TTS and microphone (STT)": "TTS e microfone (STT)",
  "Logs": "Logs",
  "Diagnostic files on disk": "Arquivos de diagnóstico no disco",
  "Full JSON": "JSON completo",
  "Edit entire settings.json": "Editar settings.json inteiro",
  "History": "Histórico",
  "Past sessions and ratings": "Sessões passadas e ratings",
  "Diagnostics": "Diagnóstico",
  "Live state and recent logs": "Estado ao vivo e logs recentes",
};

function walk(obj, map, out) {
  for (const [k, v] of Object.entries(obj)) {
    if (typeof v === "string") {
      out[k] = map[v] ?? v;
    } else if (v && typeof v === "object") {
      out[k] = {};
      walk(v, map, out[k]);
    } else {
      out[k] = v;
    }
  }
}

walk(en, ptMap, pt);

// Manual PT overrides for interpolated / complex strings
Object.assign(pt.languageGate, {
  title: "Escolha seu idioma",
  hint: "Você pode mudar isso a qualquer momento em Configuração → Idioma.",
  choose: "Selecionar idioma",
  confirm: "Continuar",
});
Object.assign(pt.control, {
  queueEmpty: "Vazia — importe na biblioteca ou peça no chat",
  readyToChat: "Pronta para conversar",
  startSession: "Começa a sessão",
  noMessages: "Nenhuma mensagem ainda",
  firstMsgHint: "Manda a primeira mensagem — a persona planeja cena, fase e duração.",
  chatHint: "Fala com a persona ou usa Autospeak para ela continuar sozinha.",
  initialPlaceholder: "Sua mensagem inicial…",
  chatPlaceholder: "Fala com ela — movimento segue a cena",
  personaThinking: "Persona pensando…",
  messageSent: "Mensagem enviada",
  stopWordTriggered: "Parada por stop word",
});
Object.assign(pt.handsFree, {
  climax: "Vou gozar",
  stopAuto: "Parar auto",
  startAuto: "Ligar auto",
  stopped: "Hands-free parado",
  started: "Auto ligado — IA enche a fila sem chat",
});
Object.assign(pt.mouse, {
  title: "Controle por mouse",
  subtitle: "Mova o mouse sobre o cilindro — posição em tempo real no Handy.",
  recording: "Gravando",
  active: "Ativo",
  starting: "Iniciando…",
  startControl: "Iniciar controle",
  connectFirst: "Conecte o Handy antes de iniciar",
  controlActive: "Controle por mouse ativo",
});
Object.assign(pt.library, {
  tabBlocks: "Blocos",
  tabImport: "Importar",
  blocksHint: "Padrões extraídos · filtre e envie para o Reprodutor",
  anyBpm: "Qualquer BPM",
  anyDuration: "Qualquer duração",
  allSpeeds: "Todas velocidades",
  roleClimax: "Gozar",
  testHandy: "Testar no Handy",
  editTrim: "Editar / recortar",
});
Object.assign(pt.player, {
  defaultQueueName: "Fila",
  signalClimax: "Gozar",
});
Object.assign(pt.settings, {
  saveBarHint: "Alterações valem após salvar. JSON completo em \"JSON completo\".",
  saveSettings: "Salvar configurações",
});

// FR overrides
Object.assign(fr.languageGate, {
  title: "Choisissez votre langue",
  hint: "Vous pouvez changer cela à tout moment dans Paramètres → Langue.",
  choose: "Sélectionner la langue",
  confirm: "Continuer",
});
Object.assign(fr.common, {
  loading: "Chargement…",
  error: "Erreur",
  save: "Enregistrer",
  delete: "Supprimer",
  cancel: "Annuler",
  close: "Fermer",
  send: "Envoyer",
  stop: "Arrêter",
});
Object.assign(fr.nav, {
  control: "Contrôle",
  handsFree: "Mains libres",
  library: "Bibliothèque",
  player: "Lecteur",
  settings: "Paramètres",
});
Object.assign(fr.config, {
  title: "Configuration",
  searchPlaceholder: "Rechercher…",
});

// RU overrides  
Object.assign(ru.languageGate, {
  title: "Выберите язык",
  hint: "Вы можете изменить это в любое время в Настройки → Язык.",
  choose: "Выбор языка",
  confirm: "Продолжить",
});
Object.assign(ru.common, {
  loading: "Загрузка…",
  error: "Ошибка",
  save: "Сохранить",
  delete: "Удалить",
  cancel: "Отмена",
  close: "Закрыть",
  send: "Отправить",
  stop: "Стоп",
});
Object.assign(ru.nav, {
  control: "Управление",
  handsFree: "Без рук",
  library: "Библиотека",
  player: "Плеер",
  settings: "Настройки",
});
Object.assign(ru.config, {
  title: "Настройки",
  searchPlaceholder: "Поиск…",
});

const localesDir = path.join(__dirname, "../src/i18n/locales");
fs.writeFileSync(path.join(localesDir, "pt.json"), JSON.stringify(pt, null, 2) + "\n");
fs.writeFileSync(path.join(localesDir, "fr.json"), JSON.stringify(fr, null, 2) + "\n");
fs.writeFileSync(path.join(localesDir, "ru.json"), JSON.stringify(ru, null, 2) + "\n");
console.log("Generated pt.json, fr.json, ru.json");
