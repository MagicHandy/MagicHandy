import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { api, AUTHENTICATION_REQUIRED_EVENT } from "../api/client";
import type { AuthenticationStatus } from "../api/types";
import { t } from "../i18n";

interface AuthValue {
  status: AuthenticationStatus | null;
  loading: boolean;
  error: string;
  refresh: () => Promise<AuthenticationStatus | null>;
  login: (username: string, password: string) => Promise<void>;
  bootstrap: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  selectControlIdentity: (accountID: string) => Promise<void>;
}

const AuthContext = createContext<AuthValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthenticationStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const generation = useRef(0);

  const refresh = useCallback(async () => {
    const current = ++generation.current;
    setLoading(true);
    try {
      const next = await api.authStatus();
      if (generation.current === current) {
        setStatus(next);
        setError("");
      }
      return next;
    } catch (reason) {
      if (generation.current === current) {
        setError(reason instanceof Error ? reason.message : t("Authentication status is unavailable."));
      }
      return null;
    } finally {
      if (generation.current === current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const authenticationRequired = () => {
      setStatus((current) => current ? {
        ...current,
        authentication_required: true,
        authenticated: false,
        account: null,
      } : current);
      void refresh();
    };
    window.addEventListener(AUTHENTICATION_REQUIRED_EVENT, authenticationRequired);
    return () => {
      generation.current += 1;
      window.removeEventListener(AUTHENTICATION_REQUIRED_EVENT, authenticationRequired);
    };
  }, [refresh]);

  const login = useCallback(async (username: string, password: string) => {
    await api.authLogin(username, password);
    await refresh();
  }, [refresh]);

  const bootstrap = useCallback(async (username: string, password: string) => {
    await api.authBootstrap(username, password);
    await refresh();
  }, [refresh]);

  const logout = useCallback(async () => {
    try {
      await api.authLogout();
    } finally {
      // An expired cookie is cleared by the backend even though logout returns
      // 401. Refresh in either case so the shell cannot stay on stale account
      // state merely because the explicit sign-out raced session expiry.
      await refresh();
    }
  }, [refresh]);

  const selectControlIdentity = useCallback(async (accountID: string) => {
    const response = await api.selectControlIdentity(accountID);
    setStatus((current) => current ? { ...current, control_identities: response.control_identities } : current);
  }, []);

  const value = useMemo<AuthValue>(() => ({
    status,
    loading,
    error,
    refresh,
    login,
    bootstrap,
    logout,
    selectControlIdentity,
  }), [bootstrap, error, loading, login, logout, refresh, selectControlIdentity, status]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthValue {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used within AuthProvider");
  return value;
}
