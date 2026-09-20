// Account, login-session and network payloads mirror the backend. Display names
// and client hints confer no authority; only backend capabilities do.

export type AccountRole = "admin" | "operator";

export interface RecoveryCodeStatus {
  remaining: number;
  limit: number;
  created_at?: string;
}

export interface IssuedRecoveryCodes extends RecoveryCodeStatus {
  codes: string[];
}

export interface ManagedSession {
  id: string;
  name: string;
  client: { browser: string; platform: string };
  created_at: string;
  last_active_at: string;
  expires_at: string;
  idle_expires_at: string;
  current: boolean;
  controller: boolean;
  device_gateway: boolean;
}

export interface ManagedSessionsResponse {
  sessions: ManagedSession[];
  current_session_id: string;
  limit: number;
}

export interface UserAccount {
  id: string;
  username: string;
  role: AccountRole;
  disabled: boolean;
  has_profile_image: boolean;
  profile_updated_at?: string;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ControlIdentity {
  account: UserAccount;
  relationship: "self" | "linked";
  label: string;
  selected: boolean;
}

export interface AuthenticationStatus {
  session_id?: string;
  capabilities?: AccountCapabilities;
  initialized: boolean;
  authentication_required: boolean;
  authenticated: boolean;
  bootstrap_available: boolean;
  ui_locale: string;
  account: UserAccount | null;
  control_identities: ControlIdentity[] | null;
}

export interface AccountCapabilities {
  control: boolean;
  configure_host: boolean;
  shared_data: boolean;
}

export interface ControlGrant {
  id: string;
  account_id: string;
  issued_by: string;
  created_at: string;
  expires_at: string | null;
}

export type ControlGrantDuration = number | "permanent";

export interface NetworkConfig {
  mode: "local" | "direct_https" | "trusted_proxy" | "legacy";
  listen_address: string;
  public_url: string;
  trusted_proxies: string[] | null;
  tls_certificate: string;
  tls_private_key: string;
}

export interface NetworkStatus {
  active: NetworkConfig;
  saved: NetworkConfig | null;
  restart_required: boolean;
  interfaces: Array<{ name: string; address: string; loopback: boolean }>;
  forwarded: boolean;
  authentication_required: boolean;
  secure_cookie: boolean;
  certificate?: { not_before: string; not_after: string; sha256: string; renewal_due: boolean; reload_error: boolean };
}
