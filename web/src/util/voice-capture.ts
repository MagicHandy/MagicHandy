export interface PCMStream {
  sampleRate: number;
  stop: () => void;
}

const PROCESSOR_NAME = "magichandy-pcm-capture";
const PROCESSOR_SOURCE = `
class MagicHandyPCMCapture extends AudioWorkletProcessor {
  constructor() {
    super();
    this.pending = new Float32Array(1024);
    this.offset = 0;
  }
  process(inputs) {
    const channels = inputs[0];
    const frames = channels && channels[0] ? channels[0].length : 0;
    for (let frame = 0; frame < frames; frame += 1) {
      let sample = 0;
      for (let channel = 0; channel < channels.length; channel += 1) sample += channels[channel][frame] || 0;
      this.pending[this.offset++] = channels.length ? sample / channels.length : 0;
      if (this.offset === this.pending.length) {
        const ready = this.pending;
        this.port.postMessage(ready, [ready.buffer]);
        this.pending = new Float32Array(1024);
        this.offset = 0;
      }
    }
    return true;
  }
}
registerProcessor("${PROCESSOR_NAME}", MagicHandyPCMCapture);
`;

export async function openPCMStream(stream: MediaStream, onSamples: (samples: Float32Array) => void): Promise<PCMStream> {
  const context = new AudioContext({ latencyHint: "interactive" });
  let moduleURL = "";
  let source: MediaStreamAudioSourceNode | undefined;
  let capture: AudioWorkletNode | undefined;
  let silence: GainNode | undefined;
  try {
    if (!context.audioWorklet) throw new Error("Continuous voice input requires AudioWorklet support.");
    moduleURL = URL.createObjectURL(new Blob([PROCESSOR_SOURCE], { type: "text/javascript" }));
    await context.audioWorklet.addModule(moduleURL);
    source = context.createMediaStreamSource(stream);
    capture = new AudioWorkletNode(context, PROCESSOR_NAME);
    silence = context.createGain();
    silence.gain.value = 0;
    capture.port.onmessage = (event: MessageEvent<unknown>) => {
      if (event.data instanceof Float32Array && event.data.length) onSamples(event.data);
    };
    source.connect(capture);
    capture.connect(silence);
    silence.connect(context.destination);
    await context.resume();
  } catch (error) {
    if (moduleURL) URL.revokeObjectURL(moduleURL);
    await context.close().catch(() => undefined);
    throw error;
  }

  let stopped = false;
  return {
    sampleRate: context.sampleRate,
    stop: () => {
      if (stopped) return;
      stopped = true;
      capture!.port.onmessage = null;
      source!.disconnect();
      capture!.disconnect();
      silence!.disconnect();
      URL.revokeObjectURL(moduleURL);
      void context.close().catch(() => undefined);
    },
  };
}
