import { useEffect, useRef, useState } from "react";
import { api, clientId } from "../api/client";
import type { NeuTTSReference } from "../api/types";
import { HostPathField } from "./HostPathField";

interface Props {
  initialCodes: string;
  initialWAV: string;
  initialTranscript: string;
  onApply: (reference: { codes: string; wav: string; transcript: string }) => void;
  onClose: () => void;
}

export function NeuTTSReferenceDialog({ initialCodes, initialWAV, initialTranscript, onApply, onClose }: Props) {
  const [sourcePath, setSourcePath] = useState(initialCodes);
  const [wavPath, setWAVPath] = useState(initialWAV);
  const [transcript, setTranscript] = useState(initialTranscript);
  const [prepared, setPrepared] = useState<NeuTTSReference | null>(null);
  const [previewURL, setPreviewURL] = useState("");
  const [preparing, setPreparing] = useState(false);
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
      if (event.key !== "Tab" || !dialogRef.current) return;
      const focusable = Array.from(dialogRef.current.querySelectorAll<HTMLElement>(
        'button:not(:disabled), input:not(:disabled), textarea:not(:disabled), audio[controls], [tabindex]:not([tabindex="-1"])',
      ));
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      request.current?.abort();
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [onClose]);

  function invalidatePrepared(clearTranscript = false) {
    request.current?.abort();
    request.current = null;
    setPrepared(null);
    setPreviewURL("");
    setError("");
    setPreparing(false);
    if (clearTranscript) setTranscript("");
  }

  async function prepare() {
    const source = sourcePath.trim();
    if (!source) {
      setError("Choose an official NeuTTS .pt tensor or an int32 .npy file.");
      return;
    }
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setPreparing(true);
    setError("");
    setPrepared(null);
    setPreviewURL("");
    try {
      const result = await api.prepareNeuTTSReference(source, wavPath.trim(), controller.signal);
      if (controller.signal.aborted) return;
      setPrepared(result.reference);
      setPreviewURL(result.preview_url);
      setTranscript((current) => current.trim() || result.reference.transcript || "");
      if (result.reference.audio_path) setWAVPath(result.reference.audio_path);
    } catch (reason) {
      if (!controller.signal.aborted) {
        setError(reason instanceof Error ? reason.message : "The reference voice could not be prepared.");
      }
    } finally {
      if (request.current === controller) request.current = null;
      if (!controller.signal.aborted) setPreparing(false);
    }
  }

  function apply() {
    if (!prepared || !previewURL || !transcript.trim()) return;
    onApply({ codes: prepared.codes_path, wav: prepared.audio_path ?? "", transcript: transcript.trim() });
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
        aria-modal="true"
        aria-labelledby="neutts-reference-title"
        aria-describedby="neutts-reference-description"
        tabIndex={-1}
      >
        <header className="reference-dialog-header">
          <div>
            <p className="eyebrow">NeuTTS Air</p>
            <h2 id="neutts-reference-title">Prepare reference voice</h2>
          </div>
          <button type="button" className="reference-dialog-close" aria-label="Close reference voice window" title="Close" onClick={onClose}>×</button>
        </header>

        <div className="reference-dialog-body">
          <p id="neutts-reference-description" className="form-status">
            Import pre-encoded NeuCodec tokens from an official sample-style <code>.pt</code> file or a one-dimensional int32 <code>.npy</code> file. MagicHandy validates the tensor without running Python or pickle code.
          </p>
          <HostPathField
            label="Reference code source (.pt or .npy)"
            kind="neutts_codes"
            value={sourcePath}
            disabled={preparing}
            onChange={(value) => { invalidatePrepared(true); setSourcePath(value); }}
          />
          <HostPathField
            label="Matching reference audio (.wav)"
            kind="wav"
            value={wavPath}
            disabled={preparing}
            placeholder="Optional when a same-name WAV is beside the code source"
            onChange={(value) => { invalidatePrepared(true); setWAVPath(value); }}
          />
          <div className="reference-prepare-row">
            <button type="button" className="btn btn-secondary" disabled={preparing || !sourcePath.trim()} onClick={() => void prepare()}>
              {preparing ? "Preparing..." : "Prepare preview"}
            </button>
            {prepared && <span className="reference-token-count" role="status">{prepared.token_count.toLocaleString()} tokens validated</span>}
          </div>
          {error && <p className="form-status voice-worker-error" role="alert">{error}</p>}

          {prepared && (
            <div className="reference-transcription">
              {previewSource ? (
                <audio key={previewSource} controls preload="metadata" src={previewSource} aria-label="NeuTTS reference audio preview" />
              ) : (
                <p className="form-status voice-worker-error" role="alert">No matching WAV was found. Choose the audio used to create these codes, then prepare the preview again.</p>
              )}
              <div className="reference-guide">
                <strong>Transcription guide</strong>
                <ol>
                  <li>Play the whole clip and listen for every spoken word.</li>
                  <li>Enter the words exactly as spoken, preserving contractions and numbers as heard.</li>
                  <li>Do not add speaker labels, stage directions, or words that are not in the audio.</li>
                </ol>
              </div>
              <label className="field">
                <span className="label">Exact reference transcript</span>
                <textarea
                  rows={4}
                  value={transcript}
                  disabled={preparing}
                  onChange={(event) => setTranscript(event.target.value)}
                  placeholder="Type exactly what the reference speaker says"
                />
              </label>
            </div>
          )}
        </div>

        <footer className="reference-dialog-actions">
          <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button type="button" className="btn btn-primary" disabled={!prepared || !previewURL || !transcript.trim()} onClick={apply}>Use reference</button>
        </footer>
      </section>
    </div>
  );
}
