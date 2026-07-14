const ASR_SAMPLE_RATE = 16000;

export async function recordingToWAV(recording: Blob, decoder?: AudioContext): Promise<Blob> {
  const context = decoder ?? new AudioContext();
  try {
    const decoded = await context.decodeAudioData(await recording.arrayBuffer());
    const frameCount = Math.max(1, Math.ceil(decoded.duration * ASR_SAMPLE_RATE));
    const resampler = new OfflineAudioContext(1, frameCount, ASR_SAMPLE_RATE);
    const source = resampler.createBufferSource();
    // Web Audio performs filtered sample-rate conversion and its standard
    // channel downmix without a browser-rate JavaScript copy.
    source.buffer = decoded;
    source.connect(resampler.destination);
    source.start();
    const rendered = await resampler.startRendering();
    const wav = encodePCM16WAV([rendered.getChannelData(0)], ASR_SAMPLE_RATE);
    const payload = new ArrayBuffer(wav.byteLength);
    new Uint8Array(payload).set(wav);
    return new Blob([payload], { type: "audio/wav" });
  } catch (error) {
    const detail = error instanceof Error ? `: ${error.message}` : "";
    throw new Error(`The browser could not convert the microphone recording to WAV${detail}`);
  } finally {
    if (!decoder) await context.close().catch(() => undefined);
  }
}

export function encodePCM16WAV(channels: Float32Array[], sourceSampleRate: number): Uint8Array {
  if (!channels.length || !channels[0]?.length || !Number.isFinite(sourceSampleRate) || sourceSampleRate <= 0) {
    throw new Error("Recorded audio contains no samples.");
  }
  const sourceFrames = Math.min(...channels.map((channel) => channel.length));
  const outputFrames = Math.max(1, Math.round(sourceFrames * ASR_SAMPLE_RATE / sourceSampleRate));
  const bytes = new Uint8Array(44 + outputFrames * 2);
  const view = new DataView(bytes.buffer);

  writeASCII(bytes, 0, "RIFF");
  view.setUint32(4, bytes.length - 8, true);
  writeASCII(bytes, 8, "WAVE");
  writeASCII(bytes, 12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, ASR_SAMPLE_RATE, true);
  view.setUint32(28, ASR_SAMPLE_RATE * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  writeASCII(bytes, 36, "data");
  view.setUint32(40, outputFrames * 2, true);

  const sourceStep = sourceSampleRate / ASR_SAMPLE_RATE;
  for (let frame = 0; frame < outputFrames; frame += 1) {
    const position = Math.min(frame * sourceStep, sourceFrames - 1);
    const before = Math.floor(position);
    const after = Math.min(before + 1, sourceFrames - 1);
    const fraction = position - before;
    let sample = 0;
    for (const channel of channels) {
      sample += channel[before] + (channel[after] - channel[before]) * fraction;
    }
    sample = Math.max(-1, Math.min(1, sample / channels.length));
    view.setInt16(44 + frame * 2, sample < 0 ? Math.round(sample * 32768) : Math.round(sample * 32767), true);
  }
  return bytes;
}

function writeASCII(target: Uint8Array, offset: number, value: string) {
  for (let index = 0; index < value.length; index += 1) target[offset + index] = value.charCodeAt(index);
}
