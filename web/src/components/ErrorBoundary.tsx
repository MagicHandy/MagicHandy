import { Component, useCallback, useEffect, useState, type ErrorInfo, type ReactNode } from "react";
import { api } from "../api/client";
import { RefreshIcon, StopIcon } from "../shell/icons";
import { stopAllAudioPlayback } from "../util/audio";

interface Props {
  children: ReactNode;
  application?: boolean;
}

interface State {
  message: string;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { message: "" };

  static getDerivedStateFromError(error: Error): State {
    return { message: error.message || "Unexpected UI error" };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("MagicHandy UI error", error, info.componentStack);
  }

  render() {
    if (this.state.message) {
      if (this.props.application) {
        return <ApplicationFailure message={this.state.message} />;
      }
      return (
        <section className="panel" role="alert">
          <h2 className="section-title">This view could not render</h2>
          <p className="form-status">{this.state.message}</p>
          <button type="button" className="btn btn-secondary" onClick={() => this.setState({ message: "" })}>
            Try again
          </button>
        </section>
      );
    }
    return this.props.children;
  }
}

function ApplicationFailure({ message }: { message: string }) {
  const [stopStatus, setStopStatus] = useState("");
  const stop = useCallback(async () => {
    stopAllAudioPlayback();
    window.dispatchEvent(new Event("magichandy:emergency-stop"));
    setStopStatus("Sending Stop...");
    try {
      const result = await api.stopMotion();
      setStopStatus(result?.error || "Stop request sent.");
    } catch {
      setStopStatus("Stop request failed. Check the device connection.");
    }
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      void stop();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [stop]);

  return (
    <main className="fatal-screen" role="alert">
      <h1>MagicHandy could not finish loading</h1>
      <p>{message}</p>
      <div className="fatal-actions">
        <button type="button" className="btn btn-danger" aria-label="Emergency stop all motion" onClick={() => void stop()}>
          <StopIcon /> Emergency Stop <span className="kbd" aria-hidden="true">Esc</span>
        </button>
        <button type="button" className="btn btn-secondary" onClick={() => window.location.reload()}>
          <RefreshIcon /> Reload application
        </button>
      </div>
      {stopStatus && <p role="status">{stopStatus}</p>}
    </main>
  );
}
