import { useState } from "react";
import type {
  ConnectionCheckResult,
  LLMModelManagerSnapshot,
  NetworkStatus,
  PublicSettings,
  SetupJob,
  SetupStatus,
} from "../api/types";
import { DismissibleNotice } from "./DismissibleNotice";
import { SetupQwenReference, type SetupQwenReferenceProps } from "./SetupQwenReference";
import { AccessScopeChoices, configForScope, NetworkPortChecklist, type NetworkScope } from "./NetworkSetupFields";
import { NetworkSettingsPanel } from "./NetworkSettingsPanel";
import { PasswordConfirmationField } from "./PasswordConfirmationField";
import { handyModelDetail, handyModelLabel } from "./QuickSettings";
import type { RuntimeChoice } from "./SetupChatStep";
import { SetupChoice as Choice } from "./SetupChoice";
import { SetupFailureReport } from "./SetupFailureReport";
import { LOCALE_OPTIONS, t, translateKnown } from "../i18n";

// Presentational steps of the setup wizard. SetupRoute owns the state,
// ordering and backend calls; these render one step each.

export type VoiceChoice = "none" | "faster-qwen3-tts" | "chatterbox" | "external";
export type AccessChoice = "local" | "protected";
export const HANDY_MODELS = ["handy_original", "handy_2_standard", "handy_2_pro"] as const;
export type HandyModel = (typeof HANDY_MODELS)[number];

export const activeSetupJob = (job?: SetupJob) => job?.status === "queued" || job?.status === "running";

export function AccessStep({
  choice,
  initialized,
  username,
  password,
  confirmation,
  locked,
  setChoice,
  setUsername,
  setPassword,
  setConfirmation,
  scope, setScope, networkRequired, networkLoaded, networkStatus, onNetworkReady, backendOnline,
}: {
  choice: AccessChoice;
  initialized: boolean;
  username: string;
  password: string;
  confirmation: string;
  locked: boolean;
  setChoice: (choice: AccessChoice) => void;
  setUsername: (value: string) => void;
  setPassword: (value: string) => void;
  setConfirmation: (value: string) => void;
  scope: NetworkScope;
  setScope: (scope: NetworkScope) => void;
  networkRequired: boolean;
  networkLoaded: boolean;
  networkStatus: NetworkStatus | null;
  onNetworkReady: (ready: boolean) => void;
  backendOnline: boolean;
}) {
  // Most installs stay on this computer, so remote setup starts folded away.
  // A saved remote configuration opens it, and it can always be set up later.
  const [remoteOpen, setRemoteOpen] = useState(false);
  const showRemote = remoteOpen || networkRequired;
  return <div className="setup-copy">
    {showRemote
      ? <AccessScopeChoices value={scope} disabled={locked || !networkLoaded} onChange={setScope} />
      : <div className="setup-access-local">
        <p>{t("MagicHandy opens only on this computer. No ports, certificates, or sign-in are needed.")}</p>
        <button type="button" className="btn btn-quiet setup-remote-toggle" disabled={locked || !networkLoaded} onClick={() => setRemoteOpen(true)}>{t("Use MagicHandy from a phone or another computer")}</button>
        <p className="hint-block">{t("You can also set up remote access later in Settings > Access.")}</p>
      </div>}
    {!initialized && scope !== "local" && networkStatus && <NetworkPortChecklist draft={configForScope(scope, networkStatus.saved ?? networkStatus.active, networkStatus)} />}
    {scope === "local" && !initialized && <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={choice === "protected"} disabled={locked} onChange={(event) => setChoice(event.target.checked ? "protected" : "local")} /><span className="track" aria-hidden="true" /></span><span>{t("Require an account and password")}</span></label>}
    {initialized ? <DismissibleNotice id="setup-protection" className="setup-notice"><strong>{t("Password protection is active.")}</strong><span>{t("Manage accounts, passwords, and your profile image from Settings > Access.")}</span></DismissibleNotice> : choice === "protected" && <div className="setup-subsection account-setup-fields">
      <label className="field"><span className="label">{t("Administrator username")}</span><input type="text" autoComplete="username" spellCheck={false} value={username} disabled={locked} onChange={(event) => setUsername(event.target.value)} /></label>
      <div className="setup-fields two-columns">
        <label className="field"><span className="label">{t("Password")}</span><input type="password" autoComplete="new-password" value={password} disabled={locked} onChange={(event) => setPassword(event.target.value)} /><span className="hint">{t("At least 15 characters. A long, unique passphrase is recommended.")}</span></label>
        <PasswordConfirmationField password={password} confirmation={confirmation} disabled={locked} onChange={setConfirmation} />
      </div>
      <p className="hint-block">{t("The password goes directly to the local account API. It is never written to installer logs, command lines, response files, or settings.")}</p>
    </div>}
    {networkRequired && (initialized ? <NetworkSettingsPanel key={scope} backendOnline={backendOnline && !locked} administrator initialScope={scope} onReadyChange={onNetworkReady} /> : <p className="hint-block">{t("Continue to create the administrator, then set up HTTPS here. Remote access stays off until the certificate is ready and you save and restart.")}</p>)}
  </div>;
}

export function WelcomeStep({ settings, patch, locked = false }: { settings: PublicSettings; patch: (patch: Partial<PublicSettings>) => void; locked?: boolean }) {
  const locale = settings.ui?.locale ?? "en";
  const chatLocale = promptLocale(settings.llm.prompt_set, locale);
  return <div className="setup-copy">
    <p>{t("Setup configures local services and optional models. Nothing downloads, builds, connects, or moves the device without a separate action.")}</p>
    <div className="setup-fields two-columns">
      <label className="field"><span className="label">{t("App language")}</span><select disabled={locked} value={locale} onChange={(event) => patch({ ui: { ...settings.ui, locale: event.target.value } })}>{LOCALE_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label>
      <label className="field"><span className="label">{t("Chat reply language")}</span><select disabled={locked} value={chatLocale} onChange={(event) => patch({ llm: { ...settings.llm, prompt_set: promptSet(event.target.value) } })}>{LOCALE_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></label>
    </div>
    <DismissibleNotice id="setup-device-safety" className="setup-notice"><strong>{t("Device safety remains active during setup.")}</strong><span>{t("Emergency Stop stays available. Connection checks never command motion.")}</span></DismissibleNotice>
  </div>;
}

export function DeviceStep({ settings, handyModel, connectionKey, connectionResult, locked, setHandyModel, setConnectionKey, patchOwner, verifyCloud }: {
  settings: PublicSettings; handyModel: HandyModel; connectionKey: string; connectionResult: ConnectionCheckResult | null; locked: boolean;
  setHandyModel: (model: HandyModel) => void; setConnectionKey: (value: string) => void; patchOwner: (owner: string) => void; verifyCloud: () => void;
}) {
  const owner = settings.device.hsp_dispatch_owner;
  return <div className="setup-copy">
    <p>{t("Choose one connection owner. You can switch later from the connection manager in the top bar.")}</p>
    <div className="setup-choices">
      <Choice selected={owner === "cloud_rest"} title={t("Handy Cloud REST")} detail={t("Recommended for firmware v4. Requires your private connection key and internet access.")} onSelect={() => patchOwner("cloud_rest")} />
      <Choice selected={owner === "browser_bluetooth"} title={t("Browser Bluetooth")} detail={t("Connect directly from a compatible browser. No cloud connection key is stored.")} onSelect={() => patchOwner("browser_bluetooth")} />
      <Choice selected={owner === "intiface"} title={t("Intiface Central")} detail={t("Use an existing Intiface Central session for local device control.")} onSelect={() => patchOwner("intiface")} />
    </div>
    {owner === "cloud_rest" && <div className="setup-subsection">
      <label className="field"><span className="label">{t("Handy connection key")}</span><input type="password" autoComplete="off" value={connectionKey} placeholder={settings.device.connection_key_set ? t("Saved key will be kept") : t("Enter connection key")} onChange={(event) => setConnectionKey(event.target.value)} /><span className="hint">{t("Connect your Handy in the official Handy Onboarding app; the key appears in the middle of its screen, under the picture of the device.")}</span></label>
      <div className="row-actions"><button type="button" className="btn btn-secondary" disabled={locked || (!connectionKey.trim() && !settings.device.connection_key_set)} onClick={verifyCloud}>{t("Save and check connection")}</button>{connectionResult && <span className="form-status" data-ok={connectionResult.ok}>{connectionResult.ok ? t("Connection verified") : translateKnown(connectionResult.message || connectionResult.status)}</span>}</div>
    </div>}
    {owner !== "cloud_rest" && <p className="hint-block">{t("Finish setup, then use the connection manager in the top bar to authorize and select the device.")}</p>}
    <h2>{t("Which Handy do you have?")}</h2>
    <p>{t("This sets the real stroke length and speed range that every mode works within.")}</p>
    <div className="setup-choices setup-choices-inline" role="radiogroup" aria-label={t("Handy model")}>
      {HANDY_MODELS.map((model) => <Choice key={model} selected={handyModel === model} title={handyModelLabel(model)} detail={handyModelDetail(model)} onSelect={() => setHandyModel(model)} />)}
    </div>
  </div>;
}

export function VoiceStep({ setup, choice, device, autoLaunch, enableAfterInstall, parakeetSelected, locked, qwenReference, setChoice, setDevice, setAutoLaunch, setEnableAfterInstall, setParakeetSelected }: {
  setup: SetupStatus; choice: VoiceChoice; device: "cpu" | "cuda"; autoLaunch: boolean; enableAfterInstall: boolean; parakeetSelected: boolean; locked: boolean;
  setChoice: (choice: VoiceChoice) => void; setDevice: (device: "cpu" | "cuda") => void; setAutoLaunch: (enabled: boolean) => void;
  setEnableAfterInstall: (enabled: boolean) => void; setParakeetSelected: (selected: boolean) => void;
  qwenReference?: SetupQwenReferenceProps;
}) {
  const module = setup.voice_modules.find((item) => item.id === choice);
  const canTurnOn = Boolean(module?.ready_after_install) || parakeetSelected || Boolean(choice === "faster-qwen3-tts" && qwenReference && !qwenReference.later && qwenReference.wav.trim() && qwenReference.transcript.trim());
  return <div className="setup-copy">
    <p>{t("Voice is optional. Modules that need no reference voice can turn on as soon as they finish installing.")}</p>
    <h2>{t("Speech output")}</h2>
    <div className="setup-choices">
      <Choice selected={choice === "none"} title={t("No speech output")} detail={t("Use text chat only. This uses no model storage or VRAM.")} onSelect={() => setChoice("none")} />
      {setup.voice_modules.map((item) => <Choice key={item.id} selected={choice === item.id} title={item.name} detail={`${translateKnown(item.summary)} ${translateKnown(item.reference_requirement)}`} badge={item.ready_after_install ? t("Ready after install") : item.recommended_for_nvidia ? t("Recommended for NVIDIA") : undefined} disabled={item.id === "faster-qwen3-tts" && !setup.hardware.nvidia} onSelect={() => setChoice(item.id as VoiceChoice)} />)}
      <Choice selected={choice === "external"} title={t("Existing compatible voice server")} detail={t("Configure its URL, model, and optional key later in Settings > Voice.")} onSelect={() => setChoice("external")} />
    </div>
    {module && <div className="setup-subsection">
      {module.supported_devices.length > 1 && <label className="field"><span className="label">{t("Execution device")}</span><select value={device} disabled={locked} onChange={(event) => setDevice(event.target.value as typeof device)}>{module.supported_devices.map((value) => <option key={value} value={value}>{value.toUpperCase()}</option>)}</select></label>}
      <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={autoLaunch} disabled={locked} onChange={(event) => setAutoLaunch(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>{t("Launch the local voice server with MagicHandy")}<small>{module.disk_estimate}</small></span></label>
      <p className="hint-block">{t("Code license: {code}. Model license: {model}.", { code: module.license, model: module.model_license })}</p>
      {choice === "faster-qwen3-tts" && qwenReference ? <SetupQwenReference {...qwenReference} /> : !module.ready_after_install && <p className="hint-block">{t("This voice needs a reference WAV and its exact transcript before it can speak. Add them in Settings > Voice, then press Start.")}</p>}
      <p className="setup-selection-state">{t("Selected for installation on the next step.")}</p>
    </div>}
    <div className="setup-divider" />
    <h2>{t("Speech input")}</h2>
    <div className="setup-choices">
      <Choice selected={!parakeetSelected} title={t("No speech input")} detail={t("Keep microphone transcription off. It can be added later in Voice settings.")} onSelect={() => setParakeetSelected(false)} />
      <Choice selected={parakeetSelected} title={setup.parakeet.name} detail={`${setup.parakeet.summary} ${t("Download: {size}.", { size: setup.parakeet.download_size })}`} onSelect={() => setParakeetSelected(true)} />
    </div>
    {parakeetSelected && <p className="hint-block">{t("Runner license: {runner}; model license: {model}.", { runner: setup.parakeet.runner_license, model: setup.parakeet.model_license })}</p>}
    {canTurnOn && <label className="toggle-line"><span className="toggle"><input type="checkbox" checked={enableAfterInstall} disabled={locked} onChange={(event) => setEnableAfterInstall(event.target.checked)} /><span className="track" aria-hidden="true" /></span><span>{t("Turn voice on when installation finishes")}<small>{t("Spoken replies and the microphone start working without a trip to Settings.")}</small></span></label>}
  </div>;
}

export function InstallStep({ job, submitted, runtimeChoice, voiceChoice, parakeetSelected, cancel, retry }: {
  job?: SetupJob; submitted: boolean; runtimeChoice: RuntimeChoice; voiceChoice: VoiceChoice; parakeetSelected: boolean;
  cancel: () => void; retry: () => void;
}) {
  if (!submitted) return <div className="setup-copy"><p>{t("Preparing the installation plan...")}</p></div>;
  if (!job) return <div className="setup-copy">
    <p>{t("No selected component needs installation. Existing services and skipped features were left unchanged.")}</p>
    <dl className="setup-summary">
      <div><dt>{t("Model runtime")}</dt><dd>{translateKnown(runtimeChoice)}</dd></div>
      <div><dt>{t("Speech output")}</dt><dd>{translateKnown(voiceChoice)}</dd></div>
      <div><dt>{t("Speech input")}</dt><dd>{parakeetSelected ? t("Parakeet") : t("Not selected")}</dd></div>
    </dl>
  </div>;
  return <div className="setup-copy">
    <p>{t("MagicHandy is installing and verifying the selected local components. You can leave this page open; the backend owns the queue.")}</p>
    <SetupJobPanel job={job} cancel={cancel} />
    {(job.status === "failed" || job.status === "cancelled") && <button type="button" className="btn btn-primary" onClick={retry}>{t("Retry installation")}</button>}
  </div>;
}

export function FinishStep({ setup, settings, models, runtimeChoice, modelPending, voiceChoice, parakeetSelected, requiresSignIn }: {
  setup: SetupStatus; settings: PublicSettings; models: LLMModelManagerSnapshot | null;
  runtimeChoice: RuntimeChoice; modelPending: boolean; voiceChoice: VoiceChoice; parakeetSelected: boolean; requiresSignIn: boolean;
}) {
  const selectedModel = models?.models.find((model) => model.id === settings.llm.model);
  const runtimeSummary = runtimeChoice === "skip"
    ? t("Skipped; chat and Autopilot remain unavailable")
    : runtimeChoice === "managed"
      ? modelPending
        ? t("Managed llama.cpp is installed. Add a model in Settings > Chat > Model to start chatting.")
        : t("Managed llama.cpp, verified with {model}", { model: selectedModel?.display_name || settings.llm.model })
      : runtimeChoice === "hosted"
        ? `${settings.llm.connections?.find(connection => connection.id === settings.llm.conversation_connection_id)?.name ?? t("Hosted model")} | ${settings.llm.connections?.find(connection => connection.id === settings.llm.conversation_connection_id)?.model ?? ""}`
        : `${runtimeChoice === "ollama" ? "Ollama" : "External llama.cpp"} | ${settings.llm.model}`;
  return <div className="setup-copy">
    <p>{t("Your choices are saved. Skipped features remain available from Settings without rerunning the Windows installer.")}</p>
    <dl className="setup-summary">
      <div><dt>{t("Data folder")}</dt><dd>{setup.data_dir}</dd></div>
      <div><dt>{t("Model runtime")}</dt><dd>{runtimeSummary}</dd></div>
      <div><dt>{t("Speech output")}</dt><dd>{translateKnown(voiceChoice)}</dd></div>
      <div><dt>{t("Speech input")}</dt><dd>{parakeetSelected ? t("Parakeet installed") : t("Not selected")}</dd></div>
      {settings.voice?.enabled && <div><dt>{t("Voice")}</dt><dd>{settings.voice.speak_replies ? t("On, speaking chat replies") : t("On")}</dd></div>}
      <div><dt>{t("Local address")}</dt><dd>{window.location.origin}</dd></div>
    </dl>
    {voiceChoice === "faster-qwen3-tts" && <p className="setup-notice">{settings.voice?.tts_reference_wav && settings.voice?.tts_reference_text
      ? t("Your Qwen voice sample and transcript are saved. You can test or change the voice in Settings > Voice.")
      : t("Qwen3-TTS is installed. Spoken replies are off until you add an audio sample and its exact transcript in Settings > Voice.")}</p>}
    {requiresSignIn && <div className="setup-notice"><strong>{t("Sign-in required after setup")}</strong><span>{t("Finishing setup ends the temporary setup session. Sign in with the administrator password you just created.")}</span></div>}
    <DismissibleNotice id="setup-before-motion" className="setup-notice"><strong>{t("Before commanding motion")}</strong><span>{t("Connect The Handy, confirm the active transport, and review speed and stroke limits in the top-bar connection manager.")}</span></DismissibleNotice>
  </div>;
}

function SetupJobPanel({ job, cancel }: { job: SetupJob; cancel: () => void }) {
  const active = activeSetupJob(job);
  const completed = job.completed_steps ?? 0;
  const total = Math.max(job.total_steps ?? job.steps?.length ?? 0, 1);
  return <section className="setup-job" aria-live="polite" aria-busy={active}>
    <div><span className="status-dot" data-state={job.status === "complete" ? "ok" : job.status === "failed" ? "error" : active ? "working" : "idle"} /><strong>{translateKnown(job.message)}</strong></div>
    <progress className="setup-install-progress" max={total} value={Math.min(completed, total)} aria-label={t("Installation progress")} />
    {active && Boolean(job.bytes_total) && <progress className="setup-install-progress" max={job.bytes_total} value={Math.min(job.bytes_completed ?? 0, job.bytes_total ?? 0)} aria-label={t("Download progress")} />}
    {job.steps && <ol className="setup-install-steps">{job.steps.map((item) => <li key={item.id} data-state={item.status}><span className="status-dot" data-state={item.status === "complete" ? "ok" : item.status === "failed" ? "error" : item.status === "running" ? "working" : "idle"} /><span><strong>{item.label}</strong>{item.message && <small>{translateKnown(item.message)}</small>}</span></li>)}</ol>}
    <div className="setup-terminal" role="log" aria-label={t("Installation terminal output")}><pre>{job.output || t("Waiting for installer output...")}</pre></div>
    <SetupFailureReport job={job} />
    {active && <button type="button" className="btn btn-secondary" onClick={cancel}>{t("Cancel installation")}</button>}
  </section>;
}

export function promptSet(locale: string): string {
  return ({
    en: "magichandy_motion_v1",
    es: "magichandy_motion_v1_es",
    "pt-BR": "magichandy_motion_v1_pt_br",
    "zh-Hans": "magichandy_motion_v1_zh_hans",
    ja: "magichandy_motion_v1_ja",
  } as Record<string, string>)[locale] ?? "magichandy_motion_v1";
}

export function promptLocale(value: string, fallback: string): string {
  return ({
    magichandy_motion_v1: "en",
    magichandy_motion_v1_es: "es",
    magichandy_motion_v1_pt_br: "pt-BR",
    magichandy_motion_v1_zh_hans: "zh-Hans",
    magichandy_motion_v1_ja: "ja",
  } as Record<string, string>)[value] ?? fallback;
}
