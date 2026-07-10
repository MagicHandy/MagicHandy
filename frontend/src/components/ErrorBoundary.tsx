import { Component, type ErrorInfo, type ReactNode } from "react";
import { withTranslation, type WithTranslation } from "react-i18next";

interface Props extends WithTranslation {
  children: ReactNode;
}

interface State {
  message: string;
}

class ErrorBoundaryBase extends Component<Props, State> {
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
          <h2 className="section-title">{this.props.t("errorBoundary.title")}</h2>
          <p className="hint">{this.state.message}</p>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => this.setState({ message: "" })}
          >
            {this.props.t("errorBoundary.retry")}
          </button>
        </section>
      );
    }
    return this.props.children;
  }
}

export const ErrorBoundary = withTranslation()(ErrorBoundaryBase);
