import { useEffect, useRef, useState } from "react";
import { api, clientId } from "../api/client";
import type { NeuTTSReference } from "../api/types";
import { trapModalTab } from "../util/modal";
import { HostPathField } from "./HostPathField";

interface Props {
  initialWAV: string;
  initialTranscript: string;
  onApply: (reference: { codes: string; wav: string; transcript: string }) => void;
  onClose: () => void;
}

export function NeuTTSReferenceDialog({ initialWAV, initialTranscript, onApply, onClose }: Props) {
  const [wavPath, setWAVPath] = useState(initialWAV);
  const [transcript, setTranscript] = useState(initialTranscript);
  const [generated, setGenerated] = useState<NeuTTSReference | null>(null);
  const [previewURL, setPreviewURL] = useState("");
  const [generating, setGenerating] = useState(false);
  const [error, setError] = useState("");
  const dialogRef = useRef<HTMLElement>(null);
  const request = useRef<AbortController | null>(null);

  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    dialogRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
        return;
      }
      if (dialogRef.current) trapModalTab(event, dialogRef.current);
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      request.current?.abort();
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [onClose]);

  function invalidateAudio() {
    request.current?.abort();
    request.current = null;
    setGenerated(null);
    setPreviewURL("");
    setError("");
    setGenerating(false);
  }

  async function generate() {
    const wav = wavPath.trim();
    const exactTranscript = transcript.trim();
    if (!wav) {
      setError("Choose the source WAV used for this reference voice.");
      return;
    }
    if (!exactTranscript) {
      setError("Enter the exact words spoken in the source WAV.");
      return;
    }
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setGenerating(true);
    setError("");
    setGenerated(null);
    setPreviewURL("");
    try {
      const result = await api.generateNeuTTSReference(wav, exactTranscript, controller.signal);
      if (controller.signal.aborted) return;
      setGenerated(result.reference);
      setPreviewURL(result.preview_url);
    } catch (reason) {
      if (!controller.signal.aborted) {
        setError(reason instanceof Error ? reason.message : "The reference voice could not be generated.");
      }
    } finally {
      if (request.current === controller) request.current = null;
      if (!controller.signal.aborted) setGenerating(false);
    }
  }

  function apply() {
    if (!generated || !previewURL || !transcript.trim()) return;
    onApply({ codes: generated.codes_path, wav: generated.audio_path ?? "", transcript: transcript.trim() });
    onClose();
  }

  const previewSource = previewURL
    ? `${previewURL}?client_id=${encodeURIComponent(clientId)}`
    : "";

  return (
    <div className="modal-scrim" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section
        ref={dialogRef}
        className="reference-dialog"
        role="dialog"
        aria-labelledby="neutts-reference-title"
        aria-describedby="neutts-reference-description"
        tabIndex={-1}
      >
        <header className="reference-dialog-header">
          <div>
            <p className="eyebrow">NeuTTS Air</p>
            <h2 id="neutts-reference-title">Create reference voice</h2>
          </div>
          <button type="button" className="reference-dialog-close" aria-label="Close reference voice window" title="Close" onClick={onClose}>{"\u00d7"}</button>
        </header>

        <div className="reference-dialog-body">
          <p id="neutts-reference-description" className="form-status">
            Generate NeuCodec reference codes locally from a WAV and its exact transcript. Python is not used.
          </p>
          <HostPathField
            label="Source voice (.wav)"
            kind="wav"
            value={wavPath}
            disabled={generating}
            placeholder="Choose a clean 3-15 second voice sample"
            onChange={(value) => { invalidateAudio(); setWAVPath(value); }}
          />
          <label className="field">
            <span className="label">Exact source transcript</span>
            <textarea
              rows={4}
              value={transcript}
              disabled={generating}
              onChange={(event) => { setTranscript(event.target.value); setError(""); }}
              placeholder="Type exactly what the speaker says"
            />
          </label>
          <div className="reference-guide">
            <strong>Reference quality</strong>
            <ul>
              <li>Use one speaker with little background noise and few long pauses.</li>
              <li>Preserve contractions and numbers exactly as spoken.</li>
              <li>Exclude speaker labels, timestamps, and stage directions.</li>
            </ul>
          </div>
          <div className="reference-prepare-row">
            <button type="button" className="btn btn-secondary" disabled={generating || !wavPath.trim() || !transcript.trim()} onClick={() => void generate()}>
              {generating ? "Generating..." : "Generate reference codes"}
            </button>
            {generated && <span className="reference-token-count" role="status">{generated.token_count.toLocaleString()} codes generated</span>}
          </div>
          {error && <p className="form-status voice-worker-error" role="alert">{error}</p>}

          {generated && previewSource && (
            <div className="reference-transcription">
              <audio key={previewSource} controls preload="metadata" src={previewSource} aria-label="NeuTTS reference audio preview" />
              <p className="form-status">Listen once before applying. Correcting the transcript does not require regenerating the audio codes.</p>
            </div>
          )}
        </div>

        <footer className="reference-dialog-actions">
          <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button type="button" className="btn btn-primary" disabled={!generated || !previewURL || !transcript.trim()} onClick={apply}>Use reference</button>
        </footer>
      </section>
    </div>
  );
}
