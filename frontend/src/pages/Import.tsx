import { useTranslation } from "react-i18next";
import { useState } from "react";
import { api, downloadText } from "../api/client";
import type { ImportResult } from "../api/types";
import { useToast } from "../hooks/useToast";

export function ImportPage() {
  const { t } = useTranslation();
  const { toast, notify } = useToast();
  const [result, setResult] = useState<ImportResult | null>(null);
  const [uploading, setUploading] = useState(false);

  const onFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const ext = file.name.toLowerCase();
    if (
      !ext.endsWith(".funscript") &&
      !ext.endsWith(".json") &&
      !ext.endsWith(".csv")
    ) {
      notify(t("library.import.invalidExt"), "error");
      return;
    }
    setUploading(true);
    try {
      const res = await api.importFile(file);
      setResult(res);
      notify(
        t("import.page.imported", { count: res.persisted?.blocks_inserted ?? 0 }),
        "ok",
      );
    } catch (err) {
      notify(err instanceof Error ? err.message : t("common.error"), "error");
    } finally {
      setUploading(false);
      e.target.value = "";
    }
  };

  const exportFile = async (fmt: string) => {
    const fileId = result?.persisted?.file_id;
    if (!fileId) {
      notify(t("import.page.importFirst"), "error");
      return;
    }
    try {
      const { filename, content } = await api.exportImport(fileId, fmt);
      downloadText(filename, content);
      notify(t("import.page.download", { filename }), "ok");
    } catch (e) {
      notify(e instanceof Error ? e.message : t("common.error"), "error");
    }
  };

  return (
    <div>
      <h2>{t("import.page.title")}</h2>
      <p className="muted">{t("import.page.hint")}</p>

      <div className="panel">
        <input
          type="file"
          accept=".funscript,.json,.csv"
          disabled={uploading}
          onChange={onFile}
        />
        {uploading && <p className="muted">{t("import.page.processing")}</p>}
      </div>

      {result && (
        <div className="panel">
          <h3 style={{ marginTop: 0 }}>{t("import.page.lastImport")}</h3>
          <p>
            {t("import.page.file")} <code>{result.source?.filename as string}</code>
          </p>
          <p>
            {t("import.page.blocksInserted", {
              count: result.persisted?.blocks_inserted ?? 0,
            })}{" "}
            <code>{result.persisted?.file_id}</code>
          </p>
          {result.summary && (
            <pre className="json-preview">
              {JSON.stringify(result.summary, null, 2)}
            </pre>
          )}
          <div className="row" style={{ marginTop: "0.75rem" }}>
            <span className="muted">{t("import.page.exportFull")}</span>
            {["funscript", "csv", "json"].map((fmt) => (
              <button
                key={fmt}
                type="button"
                className="btn secondary"
                onClick={() => exportFile(fmt)}
              >
                .{fmt}
              </button>
            ))}
          </div>
        </div>
      )}

      {toast && (
        <div className={`toast ${toast.kind === "error" ? "error" : "ok"}`}>
          {toast.text}
        </div>
      )}
    </div>
  );
}
