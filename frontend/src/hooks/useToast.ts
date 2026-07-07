import { useCallback, useState } from "react";

export function useToast() {
  const [toast, setToast] = useState<{
    text: string;
    kind: "ok" | "error";
  } | null>(null);

  const notify = useCallback((text: string, kind: "ok" | "error" = "ok") => {
    setToast({ text, kind });
    window.setTimeout(() => setToast(null), 3500);
  }, []);

  return { toast, notify };
}
