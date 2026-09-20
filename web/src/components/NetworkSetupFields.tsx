import type { InternetDiscovery, NetworkConfig, NetworkStatus } from "../api/types";
import { t } from "../i18n";

export type NetworkScope = "local" | "lan" | "public";
export const networkScope = (config: NetworkConfig): NetworkScope => config.mode === "local" || config.mode === "legacy" ? "local" : config.scope === "lan" ? "lan" : "public";

export function AccessScopeChoices({ value, disabled, onChange }: { value: NetworkScope; disabled?: boolean; onChange: (value: NetworkScope) => void }) {
  const choices: Array<{ id: NetworkScope; title: string; detail: string }> = [
    { id: "local", title: t("Local only"), detail: t("Use this computer. No open ports or certificates needed.") },
    { id: "lan", title: t("LAN + local"), detail: t("Use devices on your private network. Sign-in and local HTTPS required.") },
    { id: "public", title: t("Public"), detail: t("Use over the Internet. Sign-in, public HTTPS, and an open incoming port required.") },
  ];
  return <fieldset className="network-scope"><legend>{t("Where will you use MagicHandy?")}</legend>{choices.map((choice) => <label key={choice.id} className="network-scope-option" data-selected={choice.id === value}>
    <input type="radio" name="network-scope" value={choice.id} checked={choice.id === value} disabled={disabled} onChange={() => onChange(choice.id)} />
    <span><strong>{choice.title}</strong><span>{choice.detail}</span></span>
  </label>)}</fieldset>;
}

export function listenParts(value: string): { host: string; port: string } {
  const colon = value.lastIndexOf(":");
  return { host: value.slice(0, colon).replace(/^\[|\]$/g, ""), port: value.slice(colon + 1) || "49717" };
}
const hostPort = (host: string, port: string) => `${host.includes(":") ? `[${host}]` : host}:${port}`;

export function configForScope(scope: NetworkScope, current: NetworkConfig, status: NetworkStatus): NetworkConfig {
  if (networkScope(current) === scope && current.mode !== "legacy") return current;
  const port = listenParts(current.listen_address).port;
  const address = scope === "local" ? "127.0.0.1" : status.interfaces.find((item) => item.private)?.address ?? status.interfaces.find((item) => !item.loopback)?.address ?? "";
  const listen = address ? hostPort(address, port) : "";
  return { mode: scope === "local" ? "local" : "direct_https", listen_address: listen, public_url: scope === "lan" && listen ? `https://${listen}` : "",
    trusted_proxies: [], tls_certificate: "", tls_private_key: "", ...(scope === "local" ? {} : { scope, certificate_mode: scope === "lan" ? "local_ca" : "automatic_public" }) };
}

export function NetworkPortChecklist({ draft }: { draft: NetworkConfig }) {
  const scope = networkScope(draft);
  const { port } = listenParts(draft.listen_address);
  const automatic = draft.certificate_mode === "automatic_public";
  const localCA = draft.certificate_mode === "local_ca";
  return <>{scope !== "local" && <ol className="network-checklist">
      <li>{t("Protect access with your administrator account.")}</li>
      {automatic ? <>
        <li>{t("Open incoming TCP 443 on your router and forward it to {address}. Add an inbound firewall exclusion for MagicHandy on TCP {port}.", { address: draft.listen_address || t("the selected local address"), port })}</li>
        <li>{t("Keep this port open and MagicHandy running for automatic certificate renewal. Port 80 is not needed.")}</li>
      </> : scope === "lan" ? <>
        <li>{t("Allow incoming TCP {port} for MagicHandy on private networks in this computer's firewall. Router port forwarding is not needed.", { port })}</li>
        {localCA && <li>{t("Download the local trust certificate after setup and install it as a trusted root on each client device, including this computer. Never bypass a certificate warning.")}</li>}
      </> : <li>{t("Open the HTTPS port at your reverse proxy or host firewall. Forward only the app's HTTPS listener; keep model and voice worker ports private.")}</li>}
      <li>{t("After saving, restart MagicHandy and sign in at the displayed HTTPS address. Test from another device.")}</li>
    </ol>}</>;
}

export function NetworkSetupFields({ draft, status, locked, discovery, detecting, patch, detect }: {
  draft: NetworkConfig; status: NetworkStatus; locked: boolean; discovery: InternetDiscovery | null; detecting: boolean;
  patch: (patch: Partial<NetworkConfig>) => void; detect: () => void;
}) {
  const scope = networkScope(draft);
  const { host, port } = listenParts(draft.listen_address);
  const automatic = draft.certificate_mode === "automatic_public";
  const localCA = draft.certificate_mode === "local_ca";
  const changeListen = (value: string) => patch({ listen_address: value, ...(localCA ? { public_url: value ? `https://${value}` : "" } : {}) });
  return <>
    <NetworkPortChecklist draft={draft} />
    <div className="settings-grid two">
      <label className="field"><span className="label">{t("Listen address and port")}</span><input type="text" value={draft.listen_address} list="network-interfaces" disabled={locked} spellCheck={false} onChange={(event) => changeListen(event.target.value)} /><datalist id="network-interfaces">{status.interfaces.filter((item) => scope === "local" ? item.loopback : !item.loopback).map((item) => <option key={`${item.name}/${item.address}`} value={hostPort(item.address, port)}>{item.name}</option>)}</datalist>{scope !== "local" && <span className="hint">{t("Use this computer's network IP. Reserve it in your router so forwarding stays correct.")}</span>}</label>
      {scope !== "local" && <label className="field"><span className="label">{t("HTTPS address")}</span><input type="text" value={draft.public_url} disabled={locked || localCA} spellCheck={false} onChange={(event) => patch({ public_url: event.target.value })} /><span className="hint">{automatic ? t("Use a public IP or your domain. Public access uses TCP 443; it can forward to a different local port.") : t("The exact address clients use, including a nonstandard port. No path or credentials.")}</span></label>}
    </div>
    {automatic && <div className="network-discovery">
      <button type="button" className="btn btn-secondary" disabled={locked || detecting} onClick={detect}>{detecting ? t("Detecting address...") : t("Detect public IP and certificate service")}</button>
      <p className="hint-block">{t("Detection contacts ipify and Let's Encrypt. A detected address may belong to a VPN or shared ISP connection; it does not prove that incoming connections can reach this computer.")}</p>
      {discovery?.public_ip && <p className="form-status">{t("Detected public IP:")} <code>{discovery.public_ip}</code></p>}
      {discovery?.ip_error && <p className="form-status">{t("Public IP detection failed. You can enter your public IP or domain above.")}</p>}
      {discovery?.ca_error && <p className="form-status" role="alert">{t("Certificate service unavailable. Retry detection before setup.")}</p>}
      {(discovery?.terms_url || draft.accepted_terms) && <label className="network-terms"><input type="checkbox" checked={Boolean(draft.accepted_terms)} disabled={locked || detecting} onChange={(event) => patch({ accepted_terms: event.target.checked ? discovery?.terms_url ?? "" : "" })} /><span>{t("I agree to the certificate authority's terms.")} <a href={discovery?.terms_url || draft.accepted_terms} target="_blank" rel="noreferrer">{t("Read Let's Encrypt terms")}</a> {t("The certificate's public IP or domain is published in certificate transparency logs.")}</span></label>}
      <p className="hint-block">{t("If your ISP uses shared addressing (CGNAT) or blocks incoming TCP 443, ask it for a public address or use an existing HTTPS reverse proxy.")}</p>
    </div>}
    {scope !== "local" && <details className="network-advanced"><summary>{t("Advanced HTTPS options")}</summary>
      <label className="field"><span className="label">{t("Certificate setup")}</span><select disabled={locked} value={draft.mode === "trusted_proxy" ? "proxy" : draft.certificate_mode || "manual"} onChange={(event) => patch({ mode: event.target.value === "proxy" ? "trusted_proxy" : "direct_https", certificate_mode: event.target.value === "manual" || event.target.value === "proxy" ? undefined : event.target.value as NetworkConfig["certificate_mode"], accepted_terms: undefined, trusted_proxies: [], tls_certificate: "", tls_private_key: "", ...(event.target.value === "local_ca" ? { public_url: `https://${hostPort(host, port)}` } : {}) })}>
        <option value={scope === "lan" ? "local_ca" : "automatic_public"}>{scope === "lan" ? t("Automatic local certificate") : t("Automatic public certificate")}</option><option value="manual">{t("Use existing certificate files")}</option><option value="proxy">{t("Trusted reverse proxy")}</option>
      </select></label>
      {draft.mode === "trusted_proxy" && <label className="field"><span className="label">{t("Trusted proxy peers")}</span><input type="text" value={(draft.trusted_proxies ?? []).join(", ")} disabled={locked} onChange={(event) => patch({ trusted_proxies: event.target.value.split(",").map((part) => part.trim()) })} /><span className="hint">{t("Only these immediate proxy IPs or CIDRs may reach this listener. The proxy must replace forwarded headers.")}</span></label>}
      {draft.mode === "direct_https" && !draft.certificate_mode && <div className="settings-grid two">
        <label className="field"><span className="label">{t("Certificate chain path on this host")}</span><input type="text" value={draft.tls_certificate} disabled={locked} onChange={(event) => patch({ tls_certificate: event.target.value })} /></label>
        <label className="field"><span className="label">{t("Private key path on this host")}</span><input type="text" value={draft.tls_private_key} disabled={locked} onChange={(event) => patch({ tls_private_key: event.target.value })} /></label>
      </div>}
    </details>}
  </>;
}
