import { Component, type ErrorInfo, type ReactNode } from "react";

interface Props {
  children: ReactNode;
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
