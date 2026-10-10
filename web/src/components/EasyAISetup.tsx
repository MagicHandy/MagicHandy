import type { ModelConnection } from "../api/cloud-types";
import type { PublicSettings } from "../api/types";
import { t, translateKnown } from "../i18n";
import { SetupChoice } from "./SetupChoice";
import { SetupChoiceGroup } from "./SetupSection";
import { ModelConnectionEditor } from "./ModelConnectionEditor";
import { SavedAISetup } from "./SavedAISetup";

export type EasyAIChoice = "local" | "chatgpt" | "cloud" | "saved";

export function initialEasyAIChoice(settings?: PublicSettings["llm"]): EasyAIChoice {
  const planner = settings?.motion_planner;
  const conversation = settings?.conversation_connection_id || "local";
  if (planner && !(planner.provider === "conversation" && (planner.context_policy || "conversation") === "conversation")
    && !(conversation === "local" && planner.provider === "connection" && planner.connection_id !== "local" && planner.context_policy === "technical")) return "saved";
  if (settings?.connections?.some(connection => connection.allow_fallbacks || connection.data_collection === "allow" || connection.zero_data_retention || connection.allowed_providers?.length || connection.output_mode !== "auto")) return "saved";
  const id = setupCloudConnectionID(settings);
  const connection = settings?.connections?.find(item => item.id === id);
  return connection ? connection.provider === "chatgpt" ? "chatgpt" : "cloud" : "local";
}

export function setupCloudConnectionID(settings?: PublicSettings["llm"]): string {
  const motionID = settings?.motion_planner?.provider === "connection" ? settings.motion_planner.connection_id : "";
  return motionID && motionID !== "local" ? motionID : settings?.conversation_connection_id || "";
}

export function EasyAISetup({ choice, combine, localAvailable, connection, settings, locked, select, setCombine, setProvider, patchConnection, onReady, editCustom }: {
  choice: EasyAIChoice;
  combine: boolean;
  localAvailable: boolean;
  connection?: ModelConnection;
  settings: PublicSettings["llm"];
  locked: boolean;
  select: (choice: EasyAIChoice) => void;
  setCombine: (combine: boolean) => void;
  setProvider: (provider: ModelConnection["provider"]) => void;
  patchConnection: (change: Partial<ModelConnection>) => void;
  onReady: (ready: boolean) => void;
  editCustom: () => void;
}) {
  return <section className="setup-section" aria-labelledby="easy-ai-title">
    <h2 id="easy-ai-title">{t("Where should AI run?")}</h2>
    <SetupChoiceGroup className="easy-ai-choices" labelledBy="easy-ai-title" panel={choice === "local" ? !localAvailable && <p className="hint form-row-note">{t("Local AI is unavailable here. Continue without AI for now, or choose a cloud option.")}</p>
      : choice === "saved" ? <SavedAISetup settings={settings} locked={locked} onReady={onReady} editCustom={editCustom} />
        : <>
          {choice === "cloud" && <label className="form-row"><span className="form-row-label"><strong>{t("Cloud provider")}</strong></span><select value={connection?.provider ?? "openrouter"} disabled={locked} onChange={event => setProvider(event.target.value as ModelConnection["provider"])}>
            <option value="openrouter">{translateKnown("OpenRouter")}</option><option value="openai">{translateKnown("OpenAI API")}</option><option value="compatible">{t("Other compatible provider")}</option>
          </select></label>}
          {connection && <ModelConnectionEditor key={connection.id} connection={connection} locked={locked} patch={patchConnection} onReady={onReady} />}
          <label className="toggle-line form-row" data-disabled={!localAvailable || undefined}>
            <span className="toggle"><input type="checkbox" checked={combine} disabled={locked || !localAvailable} onChange={event => setCombine(event.target.checked)} /><span className="track" aria-hidden="true" /></span>
            <span>{t("Keep chat local; use this model for Autopilot")}<small>{!localAvailable ? t("A local AI model does not fit this computer. Use cloud only now, or configure another local server in Custom setup.") : combine ? t("Chat stays local; Autopilot varies the current motion without reading the conversation.") : t("Use this model for chat and motion. Included messages, personas and memories go to this provider.")}</small></span>
          </label>
          <div className="form-row-disclosures">
            <details><summary>{t("Context sharing details")}</summary>
              <p className="hint">{combine ? t("Chat messages, personas, and memories stay out of the cloud motion request. You can change context sharing later in Settings > Model.") : t("Included chat messages, enabled personas, and memories are sent to your selected provider. You can choose an optional local backup in the next step.")}</p>
              {combine && <p className="hint">{t("The cloud planner follows motion settings and current movement, not the words in your conversation.")}</p>}
            </details>
          </div>
          <p className="hint form-row-note">{t("Accounts and keys are saved immediately. Model choices apply when you continue.")}</p>
        </>}>
      <SetupChoice title={t("Local-only AI")} detail={t("Keep chat and motion planning on this computer. MagicHandy picks a model that fits.")} selected={choice === "local"} disabled={locked} onSelect={() => select("local")} />
      <SetupChoice title={t("ChatGPT")} badge={t("Experimental")} detail={t("Connect your ChatGPT plan, with the option to keep chat local.")} selected={choice === "chatgpt"} disabled={locked} onSelect={() => select("chatgpt")} />
      <SetupChoice title={t("API / Cloud")} badge={t("Experimental")} detail={t("Use advanced cloud models with a provider API key and separate billing.")} selected={choice === "cloud"} disabled={locked} onSelect={() => select("cloud")} />
    </SetupChoiceGroup>
  </section>;
}
