export interface VoiceActivityOptions {
  sampleRate: number;
  sensitivity: number;
  silenceMillis: number;
  maxPhraseSeconds?: number;
}

export interface VoiceActivityResult {
  level: number;
  speaking: boolean;
  segment?: Float32Array;
}

// Browser-side VAD only decides phrase boundaries. The recorded speech still
// goes to the configured backend ASR worker; no transcript or command state is
// inferred in the frontend.
export class VoiceActivitySegmenter {
  private readonly sampleRate: number;
  private readonly sensitivity: number;
  private readonly silenceFrames: number;
  private readonly maxPhraseFrames: number;
  private readonly minSpeechFrames: number;
  private readonly startFrames: number;
  private readonly preRollFrames: number;
  private readonly calibrationFrames: number;
  private readonly calibrationCeiling: number;
  private noiseFloor = 0.0015;
  private calibratedFrames = 0;
  private preRoll: Float32Array[] = [];
  private preRollLength = 0;
  private phrase: Float32Array[] = [];
  private phraseLength = 0;
  private voicedLength = 0;
  private quietLength = 0;
  private activeStreak = 0;
  private active = false;

  constructor(options: VoiceActivityOptions) {
    if (!Number.isFinite(options.sampleRate) || options.sampleRate <= 0) throw new Error("Voice input sample rate is invalid.");
    this.sampleRate = options.sampleRate;
    this.sensitivity = clamp(options.sensitivity, 1, 100);
    this.silenceFrames = Math.round(this.sampleRate * clamp(options.silenceMillis, 300, 3000) / 1000);
    this.maxPhraseFrames = Math.round(this.sampleRate * clamp(options.maxPhraseSeconds ?? 20, 5, 30));
    this.minSpeechFrames = Math.round(this.sampleRate * 0.18);
    this.startFrames = Math.round(this.sampleRate * 0.05);
    this.preRollFrames = Math.round(this.sampleRate * 0.25);
    this.calibrationFrames = Math.round(this.sampleRate * 0.25);
    this.calibrationCeiling = 0.003 + (100 - this.sensitivity) * 0.00026;
  }

  push(samples: Float32Array): VoiceActivityResult {
    if (!samples.length) return { level: 0, speaking: this.active };
    const rms = rootMeanSquare(samples);
    const level = rmsLevel(rms);
    const fixedThreshold = 10 ** ((-30 - this.sensitivity * 0.3) / 20);
    const adaptiveMultiplier = 1.8 + (100 - this.sensitivity) * 0.012;
    if (!this.active && this.calibratedFrames < this.calibrationFrames) {
      this.pushPreRoll(samples);
      const weight = this.calibratedFrames === 0 ? 1 : 0.25;
      this.noiseFloor = Math.min(
        this.calibrationCeiling,
        this.noiseFloor * (1 - weight) + rms * weight,
      );
      this.calibratedFrames += samples.length;
      const candidateSpeech = rms >= Math.max(fixedThreshold, this.calibrationCeiling * 1.5);
      this.activeStreak = candidateSpeech ? this.activeStreak + samples.length : 0;
      if (this.calibratedFrames < this.calibrationFrames || this.activeStreak < this.startFrames) {
        return { level, speaking: false };
      }
      this.active = true;
      this.phrase = this.preRoll;
      this.phraseLength = this.preRollLength;
      this.voicedLength = this.activeStreak;
      this.quietLength = 0;
      this.preRoll = [];
      this.preRollLength = 0;
      return { level, speaking: true };
    }
    const threshold = Math.max(fixedThreshold, this.noiseFloor * adaptiveMultiplier);
    const voiced = rms >= threshold;

    if (!this.active) {
      if (!voiced && rms < 0.08) this.noiseFloor = this.noiseFloor * 0.96 + rms * 0.04;
      this.pushPreRoll(samples);
      this.activeStreak = voiced ? this.activeStreak + samples.length : 0;
      if (this.activeStreak >= this.startFrames) {
        this.active = true;
        this.phrase = this.preRoll;
        this.phraseLength = this.preRollLength;
        this.voicedLength = this.activeStreak;
        this.quietLength = 0;
        this.preRoll = [];
        this.preRollLength = 0;
      }
      return { level, speaking: this.active };
    }

    this.phrase.push(samples);
    this.phraseLength += samples.length;
    if (voiced) {
      this.voicedLength += samples.length;
      this.quietLength = 0;
    } else {
      this.quietLength += samples.length;
    }
    if (this.phraseLength >= this.maxPhraseFrames ||
        (this.quietLength >= this.silenceFrames && this.voicedLength >= this.minSpeechFrames)) {
      return { level, speaking: false, segment: this.finishSegment() };
    }
    return { level, speaking: true };
  }

  flush(): Float32Array | undefined {
    if (!this.active || this.voicedLength < this.minSpeechFrames) {
      this.reset();
      return undefined;
    }
    return this.finishSegment();
  }

  reset() {
    this.preRoll = [];
    this.preRollLength = 0;
    this.phrase = [];
    this.phraseLength = 0;
    this.voicedLength = 0;
    this.quietLength = 0;
    this.activeStreak = 0;
    this.active = false;
  }

  private pushPreRoll(samples: Float32Array) {
    this.preRoll.push(samples);
    this.preRollLength += samples.length;
    while (this.preRollLength > this.preRollFrames && this.preRoll.length > 1) {
      const removed = this.preRoll.shift();
      this.preRollLength -= removed?.length ?? 0;
    }
  }

  private finishSegment(): Float32Array {
    const trailingFrames = Math.min(this.quietLength, Math.round(this.sampleRate * 0.25));
    const outputFrames = Math.max(1, this.phraseLength - this.quietLength + trailingFrames);
    const output = new Float32Array(outputFrames);
    let offset = 0;
    for (const chunk of this.phrase) {
      if (offset >= outputFrames) break;
      const length = Math.min(chunk.length, outputFrames - offset);
      output.set(chunk.subarray(0, length), offset);
      offset += length;
    }
    this.reset();
    return output;
  }
}

function rootMeanSquare(samples: Float32Array): number {
  let sum = 0;
  for (const sample of samples) sum += sample * sample;
  return Math.sqrt(sum / samples.length);
}

function rmsLevel(rms: number): number {
  if (rms <= 0) return 0;
  return clamp((20 * Math.log10(rms) + 60) / 60, 0, 1);
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}
