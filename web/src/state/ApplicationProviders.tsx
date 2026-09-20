import { App } from "../App";
import { I18nProvider } from "../i18n";
import { AppStateProvider, ToastProvider } from "./app-state";
import { useAuth } from "./auth";
import { ApplicationAudience } from "./application-audience";
import { NoticePreferencesProvider } from "./notice-preferences";

// A different login is a different data audience, even when access stays
// enabled. Remount the entire application state so pending reads, private
// settings drafts, routes and device work cannot survive the identity change.
export function ApplicationProviders() {
  const { status } = useAuth();
  const accessGranted = Boolean(status && (!status.authentication_required || status.authenticated));
  const audience = status?.authentication_required && !status.authenticated ? "signed-out" : status?.session_id ? `session:${status.session_id}` : status?.account ? `account:${status.account.id}` : "local";
  return <ApplicationAudience key={audience}><AppStateProvider enabled={accessGranted}>
    <I18nProvider fallbackLocale={status?.ui_locale}>
      <NoticePreferencesProvider enabled={Boolean(status)} scope={status?.authenticated && status.account ? "account" : "browser"}>
        <ToastProvider audience={audience}><App /></ToastProvider>
      </NoticePreferencesProvider>
    </I18nProvider>
  </AppStateProvider></ApplicationAudience>;
}
