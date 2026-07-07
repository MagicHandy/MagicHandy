import i18n from "../i18n";

interface BrowserSpeechRecognition extends EventTarget {
  lang: string;
  interimResults: boolean;
  continuous: boolean;
  maxAlternatives: number;
  onresult: ((event: { results: { [i: number]: { [j: number]: { transcript: string } } } }) => void) | null;
  onerror: ((event: { error: string }) => void) | null;
  onend: (() => void) | null;
  start(): void;
  stop(): void;
}

type SpeechRecognitionCtor = new () => BrowserSpeechRecognition;

function getRecognitionCtor(): SpeechRecognitionCtor | null {
  const w = window as Window & {
    webkitSpeechRecognition?: SpeechRecognitionCtor;
    SpeechRecognition?: SpeechRecognitionCtor;
  };
  return w.SpeechRecognition ?? w.webkitSpeechRecognition ?? null;
}

export function browserSttAvailable(): boolean {
  return getRecognitionCtor() != null;
}

export function listenPushToTalk(language: string): {
  stop: () => void;
  done: Promise<string>;
} {
  const Ctor = getRecognitionCtor();
  if (!Ctor) {
    return {
      stop: () => {},
      done: Promise.reject(new Error(i18n.t("browserStt.unsupported"))),
    };
  }

  const lang =
    language === "pt" || language.startsWith("pt")
      ? "pt-BR"
      : language === "auto"
        ? "pt-BR"
        : language;

  let resolveDone!: (value: string) => void;
  let rejectDone!: (reason: Error) => void;
  const done = new Promise<string>((resolve, reject) => {
    resolveDone = resolve;
    rejectDone = (e) => reject(e);
  });

  const rec = new Ctor();
  rec.lang = lang;
  rec.interimResults = false;
  rec.continuous = false;
  rec.maxAlternatives = 1;

  let finished = false;
  let gotResult = false;
  const finish = (fn: () => void) => {
    if (finished) return;
    finished = true;
    fn();
  };

  rec.onresult = (event) => {
    gotResult = true;
    const text = event.results[0]?.[0]?.transcript?.trim() ?? "";
    finish(() => resolveDone(text));
  };
  rec.onerror = (event) => {
    if (event.error === "aborted") {
      finish(() => resolveDone(""));
      return;
    }
    finish(() => rejectDone(new Error(event.error || "speech error")));
  };
  rec.onend = () => {
    if (!gotResult) {
      finish(() => resolveDone(""));
    }
  };

  try {
    rec.start();
  } catch (e) {
    finish(() =>
      rejectDone(e instanceof Error ? e : new Error(i18n.t("browserStt.micStartError"))),
    );
  }

  return {
    stop: () => {
      try {
        rec.stop();
      } catch {
        /* already stopped */
      }
    },
    done,
  };
}
