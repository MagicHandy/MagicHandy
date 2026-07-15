// MagicHandy persistent NeuTTS runner.
// Copyright (C) 2026 MagicHandy contributors
// SPDX-License-Identifier: GPL-3.0-only

use std::collections::{HashSet, VecDeque};
use std::io::{self, BufRead, Write};
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::sync::{mpsc, Arc, Mutex};
use std::time::Instant;

use anyhow::Context as _;
use regex::Regex;
use serde::Deserialize;

const FRAME_MAGIC: &[u8; 4] = b"MHTS";
const FRAME_READY: u8 = 1;
const FRAME_AUDIO: u8 = 2;
const FRAME_DONE: u8 = 3;
const FRAME_ERROR: u8 = 4;
const FRAME_CANCELED: u8 = 5;
const RUNNER_PROTOCOL: u32 = 1;
const HOP_LENGTH: usize = 480;
const STREAM_LOOKBACK: usize = 50;
const STREAM_LOOKFORWARD: usize = 5;
const STREAM_OVERLAP: usize = 1;
const DEFAULT_SAMPLER_SEED: u32 = 3;
const PCM_CACHE_MAX_BYTES: usize = 8 << 20;
const PCM_CACHE_MAX_ENTRIES: usize = 8;
const PCM_CACHE_MAX_ENTRY_BYTES: usize = 2 << 20;
const PCM_REPLAY_FRAME_BYTES: usize = 32 << 10;

struct Options {
    codes_path: PathBuf,
    ref_text: String,
    backbone: String,
    gguf_file: Option<String>,
    chunk_size: usize,
    espeak_path: PathBuf,
    sampler_seed: Option<u32>,
}

enum RunMode {
    Serve(Options),
    Phonemize { text: String, espeak_path: PathBuf },
}

#[derive(Deserialize)]
struct WireCommand {
    #[serde(rename = "type")]
    kind: String,
    #[serde(default)]
    id: String,
    #[serde(default)]
    text: String,
}

enum Work {
    Speak { id: String, text: String },
    Invalid(String),
    Shutdown,
}

struct PcmCacheEntry {
    text: String,
    pcm: Vec<u8>,
}

#[derive(Default)]
struct PcmCache {
    entries: VecDeque<PcmCacheEntry>,
    bytes: usize,
}

impl PcmCache {
    fn get(&mut self, text: &str) -> Option<Vec<u8>> {
        let index = self.entries.iter().position(|entry| entry.text == text)?;
        let entry = self.entries.remove(index)?;
        let pcm = entry.pcm.clone();
        self.entries.push_front(entry);
        Some(pcm)
    }

    fn insert(&mut self, text: String, pcm: Vec<u8>) {
        if pcm.is_empty() || pcm.len() > PCM_CACHE_MAX_ENTRY_BYTES {
            return;
        }
        if let Some(index) = self.entries.iter().position(|entry| entry.text == text) {
            if let Some(previous) = self.entries.remove(index) {
                self.bytes -= previous.pcm.len();
            }
        }
        while self.entries.len() >= PCM_CACHE_MAX_ENTRIES
            || self.bytes + pcm.len() > PCM_CACHE_MAX_BYTES
        {
            let Some(evicted) = self.entries.pop_back() else {
                break;
            };
            self.bytes -= evicted.pcm.len();
        }
        self.bytes += pcm.len();
        self.entries.push_front(PcmCacheEntry { text, pcm });
    }
}

struct IncrementalOverlapAdd {
    stride: usize,
    next_offset: usize,
    emitted: usize,
    sums: Vec<f32>,
    weights: Vec<f32>,
}

impl IncrementalOverlapAdd {
    fn new(stride: usize) -> anyhow::Result<Self> {
        anyhow::ensure!(stride > 0, "overlap-add stride must be positive");
        Ok(Self {
            stride,
            next_offset: 0,
            emitted: 0,
            sums: Vec::new(),
            weights: Vec::new(),
        })
    }

    fn push(&mut self, frame: &[f32]) -> anyhow::Result<Vec<f32>> {
        anyhow::ensure!(
            frame.len() >= self.stride,
            "streaming audio frame is shorter than its stride"
        );
        self.add(frame)?;
        self.next_offset = self
            .next_offset
            .checked_add(self.stride)
            .context("overlap-add offset overflow")?;
        self.drain_to(self.next_offset)
    }

    fn add(&mut self, frame: &[f32]) -> anyhow::Result<()> {
        anyhow::ensure!(!frame.is_empty(), "overlap-add requires an audio frame");
        anyhow::ensure!(
            self.next_offset >= self.emitted,
            "overlap-add offset moved behind emitted audio"
        );
        let relative_offset = self.next_offset - self.emitted;
        let required = relative_offset
            .checked_add(frame.len())
            .context("overlap-add frame size overflow")?;
        self.sums.resize(required.max(self.sums.len()), 0.0);
        self.weights.resize(required.max(self.weights.len()), 0.0);
        let denominator = (frame.len() + 1) as f32;
        for (sample_index, sample) in frame.iter().enumerate() {
            let position = (sample_index + 1) as f32 / denominator;
            let weight = 0.5 - (position - 0.5).abs();
            let index = relative_offset + sample_index;
            self.sums[index] += weight * sample;
            self.weights[index] += weight;
        }
        Ok(())
    }

    fn finish(mut self) -> anyhow::Result<Vec<f32>> {
        let end = self
            .emitted
            .checked_add(self.sums.len())
            .context("overlap-add output size overflow")?;
        self.drain_to(end)
    }

    fn drain_to(&mut self, absolute_end: usize) -> anyhow::Result<Vec<f32>> {
        anyhow::ensure!(
            absolute_end >= self.emitted,
            "overlap-add drain moved backwards"
        );
        let count = (absolute_end - self.emitted).min(self.sums.len());
        let tail_sums = self.sums.split_off(count);
        let tail_weights = self.weights.split_off(count);
        let sums = std::mem::replace(&mut self.sums, tail_sums);
        let weights = std::mem::replace(&mut self.weights, tail_weights);
        let mut output = Vec::with_capacity(count);
        for (sum, weight) in sums.into_iter().zip(weights) {
            anyhow::ensure!(weight > 0.0, "overlap-add produced an uncovered sample");
            output.push(sum / weight);
        }
        self.emitted += count;
        Ok(output)
    }
}

fn main() -> anyhow::Result<()> {
    let Some(mode) = parse_args()? else {
        return Ok(());
    };
    let options = match mode {
        RunMode::Serve(options) => options,
        RunMode::Phonemize { text, espeak_path } => {
            println!("{}", phonemize(&espeak_path, &text)?);
            return Ok(());
        }
    };

    eprintln!("[magichandy-neutts] loading backbone and codec");
    let load_started = Instant::now();
    let mut tts = neutts::download::load_from_hub_cb(
        &options.backbone,
        options.gguf_file.as_deref(),
        |_| {},
    )?;
    tts.backbone.seed = options.sampler_seed;
    let codec_backend = tts.codec.backend_name().to_string();
    let ref_codes = tts
        .load_ref_codes(&options.codes_path)
        .with_context(|| format!("load reference codes from {}", options.codes_path.display()))?;
    let ref_phones = phonemize(&options.espeak_path, &options.ref_text)
        .context("phonemize reference transcript")?;

    let canceled = Arc::new(Mutex::new(HashSet::<String>::new()));
    let work = spawn_input_reader(Arc::clone(&canceled));
    let mut pcm_cache = PcmCache::default();
    let stdout = io::stdout();
    let mut out = io::BufWriter::new(stdout.lock());
    let ready = serde_json::json!({
        "protocol": RUNNER_PROTOCOL,
        "codec": codec_backend,
        "load_ms": load_started.elapsed().as_millis(),
        "sampler_seed": options.sampler_seed,
        "pcm_cache_max_bytes": if options.sampler_seed.is_some() { PCM_CACHE_MAX_BYTES } else { 0 },
    });
    write_frame(&mut out, FRAME_READY, &serde_json::to_vec(&ready)?)?;
    eprintln!(
        "[magichandy-neutts] ready: codec={}, seed={}, cache={} MiB, load={:.2}s",
        tts.codec.backend_name(),
        sampler_seed_label(options.sampler_seed),
        if options.sampler_seed.is_some() {
            PCM_CACHE_MAX_BYTES >> 20
        } else {
            0
        },
        load_started.elapsed().as_secs_f32(),
    );

    while let Ok(command) = work.recv() {
        match command {
            Work::Shutdown => break,
            Work::Invalid(message) => write_frame(&mut out, FRAME_ERROR, message.as_bytes())?,
            Work::Speak { id, text } => {
                if id.is_empty() || text.trim().is_empty() {
                    write_frame(
                        &mut out,
                        FRAME_ERROR,
                        b"speak requires non-empty id and text",
                    )?;
                    continue;
                }
                if is_canceled(&canceled, &id) {
                    clear_canceled(&canceled, &id);
                    write_frame(&mut out, FRAME_CANCELED, &[])?;
                    continue;
                }

                if options.sampler_seed.is_some() {
                    if let Some(pcm) = pcm_cache.get(&text) {
                        let started = Instant::now();
                        for chunk in pcm.chunks(PCM_REPLAY_FRAME_BYTES) {
                            if is_canceled(&canceled, &id) {
                                break;
                            }
                            write_frame(&mut out, FRAME_AUDIO, chunk)?;
                        }
                        let was_canceled = is_canceled(&canceled, &id);
                        clear_canceled(&canceled, &id);
                        if was_canceled {
                            write_frame(&mut out, FRAME_CANCELED, &[])?;
                        } else {
                            write_frame(&mut out, FRAME_DONE, &[])?;
                            eprintln!(
                                "[magichandy-neutts] request {}: {:.3}s, {:.2}s audio, cache=hit",
                                id,
                                started.elapsed().as_secs_f32(),
                                pcm.len() as f32 / 2.0 / neutts::SAMPLE_RATE as f32,
                            );
                        }
                        continue;
                    }
                }

                let input_phones = match phonemize(&options.espeak_path, &text) {
                    Ok(value) => value,
                    Err(error) => {
                        write_frame(&mut out, FRAME_ERROR, error.to_string().as_bytes())?;
                        continue;
                    }
                };
                let prompt = neutts::tokens::build_prompt(&ref_phones, &input_phones, &ref_codes);
                let started = Instant::now();
                let backbone = &tts.backbone;
                let codec = &tts.codec;
                let mut token_cache = ref_codes.clone();
                let mut decoded_tokens = ref_codes.len();
                let mut overlap = IncrementalOverlapAdd::new(options.chunk_size * HOP_LENGTH)?;
                let mut cached_pcm = options.sampler_seed.map(|_| Vec::<u8>::new());
                let mut samples = 0usize;

                let synthesis = backbone.generate_streaming(&prompt, 2048, |piece| {
                    if is_canceled(&canceled, &id) {
                        anyhow::bail!("synthesis canceled");
                    }
                    let ids = neutts::tokens::extract_ids(piece);
                    if ids.is_empty() {
                        return Ok(());
                    }
                    token_cache.extend_from_slice(&ids);
                    while token_cache.len().saturating_sub(decoded_tokens)
                        >= options.chunk_size + STREAM_LOOKFORWARD
                    {
                        let frame = decode_stream_frame(
                            codec,
                            &token_cache,
                            decoded_tokens,
                            options.chunk_size,
                        )?;
                        let mixed = overlap.push(&frame)?;
                        let pcm = encode_pcm(&mixed);
                        write_frame(&mut out, FRAME_AUDIO, &pcm)?;
                        append_cached_pcm(&mut cached_pcm, &pcm);
                        samples += mixed.len();
                        decoded_tokens += options.chunk_size;
                    }
                    Ok(())
                });

                let synthesis = synthesis.and_then(|_| {
                    if is_canceled(&canceled, &id) {
                        anyhow::bail!("synthesis canceled");
                    }
                    if token_cache.len() > decoded_tokens {
                        let frame = decode_stream_tail(codec, &token_cache, decoded_tokens)?;
                        overlap.add(&frame)?;
                        let mixed = overlap.finish()?;
                        let pcm = encode_pcm(&mixed);
                        write_frame(&mut out, FRAME_AUDIO, &pcm)?;
                        append_cached_pcm(&mut cached_pcm, &pcm);
                        samples += mixed.len();
                    }
                    Ok(())
                });

                let was_canceled = is_canceled(&canceled, &id);
                clear_canceled(&canceled, &id);
                match synthesis {
                    _ if was_canceled => write_frame(&mut out, FRAME_CANCELED, &[])?,
                    Ok(()) if samples > 0 => {
                        if let Some(pcm) = cached_pcm {
                            pcm_cache.insert(text, pcm);
                        }
                        write_frame(&mut out, FRAME_DONE, &[])?;
                        eprintln!(
                            "[magichandy-neutts] request {}: {:.2}s, {:.2}s audio, cache=miss",
                            id,
                            started.elapsed().as_secs_f32(),
                            samples as f32 / neutts::SAMPLE_RATE as f32,
                        );
                    }
                    Ok(()) => write_frame(&mut out, FRAME_ERROR, b"synthesis returned no audio")?,
                    Err(error) => write_frame(&mut out, FRAME_ERROR, error.to_string().as_bytes())?,
                }
            }
        }
    }

    eprintln!("[magichandy-neutts] stopped");
    Ok(())
}

fn phonemize(espeak_path: &Path, text: &str) -> anyhow::Result<String> {
    if text.trim().is_empty() {
        return Ok(String::new());
    }

    enum Part {
        Text,
        Mark(String),
    }

    // Match phonemizer's default punctuation set and preserve surrounding
    // whitespace while eSpeak processes each text span independently.
    let punctuation = Regex::new(r#"(?s)(\s*(?:[;:,.!?¡¿—…"«»“”(){}\[\]]+|[\r\n]+)\s*)+"#)
        .context("compile punctuation matcher")?;
    let mut parts = Vec::<Part>::new();
    let mut chunks = Vec::<String>::new();
    let mut cursor = 0usize;
    for mark in punctuation.find_iter(text) {
        let chunk = text[cursor..mark.start()].trim();
        if !chunk.is_empty() {
            chunks.push(chunk.to_string());
            parts.push(Part::Text);
        }
        parts.push(Part::Mark(mark.as_str().to_string()));
        cursor = mark.end();
    }
    let chunk = text[cursor..].trim();
    if !chunk.is_empty() {
        chunks.push(chunk.to_string());
        parts.push(Part::Text);
    }
    if chunks.is_empty() {
        return Ok(text.split_whitespace().collect::<Vec<_>>().join(" "));
    }

    let mut child = Command::new(espeak_path)
        .args(["-q", "--ipa", "-v", "en-us"])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .with_context(|| format!("start eSpeak NG at {}", espeak_path.display()))?;
    let mut stdin = child.stdin.take().context("open eSpeak NG stdin")?;
    stdin
        .write_all((chunks.join("\n") + "\n").as_bytes())
        .context("write text to eSpeak NG")?;
    drop(stdin);
    let output = child.wait_with_output().context("wait for eSpeak NG")?;
    anyhow::ensure!(
        output.status.success(),
        "eSpeak NG failed: {}",
        String::from_utf8_lossy(&output.stderr).trim()
    );

    let stdout = String::from_utf8(output.stdout).context("eSpeak NG returned non-UTF-8 IPA")?;
    let phones = stdout
        .lines()
        .map(str::trim)
        .filter(|line| !line.is_empty())
        .map(ToOwned::to_owned)
        .collect::<Vec<_>>();
    anyhow::ensure!(
        phones.len() == chunks.len(),
        "eSpeak NG returned {} phoneme spans for {} text spans",
        phones.len(),
        chunks.len()
    );

    let mut phones = phones.into_iter();
    let mut result = String::new();
    for part in parts {
        match part {
            Part::Text => result.push_str(&phones.next().context("missing phoneme span")?),
            Part::Mark(mark) => result.push_str(&mark),
        }
    }
    Ok(result.split_whitespace().collect::<Vec<_>>().join(" "))
}

fn decode_stream_frame(
    codec: &neutts::codec::NeuCodecDecoder,
    tokens: &[i32],
    decoded_tokens: usize,
    chunk_size: usize,
) -> anyhow::Result<Vec<f32>> {
    let tokens_start = decoded_tokens.saturating_sub(STREAM_LOOKBACK + STREAM_OVERLAP);
    let tokens_end =
        (decoded_tokens + chunk_size + STREAM_LOOKFORWARD + STREAM_OVERLAP).min(tokens.len());
    let audio = codec
        .decode(&tokens[tokens_start..tokens_end])
        .context("NeuCodec streaming decode failed")?;
    let sample_start = (decoded_tokens - tokens_start) * HOP_LENGTH;
    let sample_end =
        (sample_start + (chunk_size + 2 * STREAM_OVERLAP) * HOP_LENGTH).min(audio.len());
    anyhow::ensure!(
        sample_start < sample_end,
        "NeuCodec streaming window is empty"
    );
    Ok(audio[sample_start..sample_end].to_vec())
}

fn decode_stream_tail(
    codec: &neutts::codec::NeuCodecDecoder,
    tokens: &[i32],
    decoded_tokens: usize,
) -> anyhow::Result<Vec<f32>> {
    let remaining = tokens.len().saturating_sub(decoded_tokens);
    let tokens_start = tokens
        .len()
        .saturating_sub(STREAM_LOOKBACK + STREAM_OVERLAP + remaining);
    let sample_start = tokens
        .len()
        .saturating_sub(tokens_start + remaining + STREAM_OVERLAP)
        * HOP_LENGTH;
    let audio = codec
        .decode(&tokens[tokens_start..])
        .context("NeuCodec tail decode failed")?;
    anyhow::ensure!(sample_start < audio.len(), "NeuCodec tail window is empty");
    Ok(audio[sample_start..].to_vec())
}

fn spawn_input_reader(canceled: Arc<Mutex<HashSet<String>>>) -> mpsc::Receiver<Work> {
    let (sender, receiver) = mpsc::channel();
    std::thread::spawn(move || {
        let stdin = io::stdin();
        for line in stdin.lock().lines() {
            let line = match line {
                Ok(value) => value,
                Err(error) => {
                    let _ = sender.send(Work::Invalid(format!("read command: {error}")));
                    break;
                }
            };
            let command: WireCommand = match serde_json::from_str(&line) {
                Ok(value) => value,
                Err(error) => {
                    let _ = sender.send(Work::Invalid(format!("invalid command: {error}")));
                    continue;
                }
            };
            match command.kind.as_str() {
                "speak" => {
                    if sender
                        .send(Work::Speak {
                            id: command.id,
                            text: command.text,
                        })
                        .is_err()
                    {
                        break;
                    }
                }
                "cancel" => {
                    if !command.id.is_empty() {
                        canceled
                            .lock()
                            .expect("cancellation lock poisoned")
                            .insert(command.id);
                    }
                }
                "shutdown" => {
                    let _ = sender.send(Work::Shutdown);
                    return;
                }
                other => {
                    let _ = sender.send(Work::Invalid(format!("unknown command type {other:?}")));
                }
            }
        }
        let _ = sender.send(Work::Shutdown);
    });
    receiver
}

fn is_canceled(canceled: &Mutex<HashSet<String>>, id: &str) -> bool {
    canceled
        .lock()
        .expect("cancellation lock poisoned")
        .contains(id)
}

fn clear_canceled(canceled: &Mutex<HashSet<String>>, id: &str) {
    canceled
        .lock()
        .expect("cancellation lock poisoned")
        .remove(id);
}

fn encode_pcm(samples: &[f32]) -> Vec<u8> {
    let mut pcm = vec![0u8; samples.len() * 2];
    for (index, &sample) in samples.iter().enumerate() {
        let bytes = ((sample.clamp(-1.0, 1.0) * i16::MAX as f32) as i16).to_le_bytes();
        pcm[index * 2] = bytes[0];
        pcm[index * 2 + 1] = bytes[1];
    }
    pcm
}

fn append_cached_pcm(cache: &mut Option<Vec<u8>>, pcm: &[u8]) {
    let Some(cached) = cache else {
        return;
    };
    if cached.len().saturating_add(pcm.len()) > PCM_CACHE_MAX_ENTRY_BYTES {
        *cache = None;
        return;
    }
    cached.extend_from_slice(pcm);
}

fn write_frame(out: &mut impl io::Write, kind: u8, payload: &[u8]) -> anyhow::Result<()> {
    let length = u32::try_from(payload.len()).context("runner frame is too large")?;
    out.write_all(FRAME_MAGIC)
        .context("write runner frame magic")?;
    out.write_all(&[kind]).context("write runner frame type")?;
    out.write_all(&length.to_le_bytes())
        .context("write runner frame length")?;
    out.write_all(payload)
        .context("write runner frame payload")?;
    out.flush().context("flush runner frame")?;
    Ok(())
}

fn parse_args() -> anyhow::Result<Option<RunMode>> {
    let mut serve = false;
    let mut codes_path = None;
    let mut ref_text = None;
    let mut backbone = "neuphonic/neutts-air-q4-gguf".to_string();
    let mut gguf_file = None;
    let mut chunk_size = 25usize;
    let mut espeak_path = None;
    let mut phonemize_text = None;
    let mut sampler_seed = match std::env::var("MAGICHANDY_NEUTTS_SEED") {
        Ok(value) => parse_sampler_seed(&value)?,
        Err(std::env::VarError::NotPresent) => Some(DEFAULT_SAMPLER_SEED),
        Err(error) => return Err(error).context("MAGICHANDY_NEUTTS_SEED is not valid Unicode"),
    };
    let mut args = std::env::args().skip(1);
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--serve" => serve = true,
            "--codes" | "-c" => codes_path = args.next().map(PathBuf::from),
            "--ref-text" | "-r" => ref_text = args.next(),
            "--backbone" | "-b" => backbone = args.next().unwrap_or(backbone),
            "--gguf-file" | "-g" => gguf_file = args.next(),
            "--espeak" => espeak_path = args.next().map(PathBuf::from),
            "--phonemize" => phonemize_text = args.next(),
            "--seed" => {
                sampler_seed = parse_sampler_seed(
                    &args
                        .next()
                        .context("--seed requires an unsigned integer or random")?,
                )?
            }
            "--chunk" | "-k" => {
                chunk_size = args
                    .next()
                    .as_deref()
                    .and_then(|value| value.parse().ok())
                    .unwrap_or(25)
            }
            "--help" | "-h" => {
                print_help();
                return Ok(None);
            }
            other => anyhow::bail!("unknown argument {other:?}; run with --help"),
        }
    }
    let espeak_path = resolve_espeak_path(espeak_path)?;
    if let Some(text) = phonemize_text {
        return Ok(Some(RunMode::Phonemize { text, espeak_path }));
    }
    anyhow::ensure!(serve, "--serve is required; run with --help");
    anyhow::ensure!(chunk_size > 0, "--chunk must be greater than zero");
    let codes_path = codes_path.context("--codes is required")?;
    let ref_text = resolve_reference_text(ref_text, &codes_path)?;
    anyhow::ensure!(!ref_text.trim().is_empty(), "reference transcript is empty");
    Ok(Some(RunMode::Serve(Options {
        codes_path,
        ref_text,
        backbone,
        gguf_file,
        chunk_size,
        espeak_path,
        sampler_seed,
    })))
}

fn parse_sampler_seed(value: &str) -> anyhow::Result<Option<u32>> {
    if value.eq_ignore_ascii_case("random") {
        return Ok(None);
    }
    Ok(Some(value.parse::<u32>().with_context(|| {
        format!("invalid sampler seed {value:?}; use an unsigned integer or random")
    })?))
}

fn sampler_seed_label(seed: Option<u32>) -> String {
    seed.map_or_else(|| "random".to_string(), |value| value.to_string())
}

fn resolve_espeak_path(explicit: Option<PathBuf>) -> anyhow::Result<PathBuf> {
    if let Some(path) = explicit {
        anyhow::ensure!(
            probe_espeak(&path),
            "eSpeak NG is unavailable at {}",
            path.display()
        );
        return Ok(path);
    }

    let mut candidates = Vec::<PathBuf>::new();
    if let Some(path) = std::env::var_os("MAGICHANDY_ESPEAK_PATH") {
        candidates.push(PathBuf::from(path));
    }
    if let Ok(executable) = std::env::current_exe() {
        if let Some(directory) = executable.parent() {
            candidates.push(directory.join(format!("espeak-ng{}", std::env::consts::EXE_SUFFIX)));
        }
    }
    if cfg!(windows) {
        for variable in ["ProgramFiles", "ProgramFiles(x86)"] {
            if let Some(root) = std::env::var_os(variable) {
                candidates.push(PathBuf::from(root).join("eSpeak NG").join("espeak-ng.exe"));
            }
        }
    }
    candidates.push(PathBuf::from(format!(
        "espeak-ng{}",
        std::env::consts::EXE_SUFFIX
    )));
    for candidate in candidates {
        if probe_espeak(&candidate) {
            return Ok(candidate);
        }
    }
    anyhow::bail!(
        "eSpeak NG 1.52 is required for NeuTTS phonemization; rerun the MagicHandy installer"
    )
}

fn probe_espeak(path: &Path) -> bool {
    Command::new(path)
        .arg("--version")
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .is_ok_and(|status| status.success())
}

fn resolve_reference_text(
    value: Option<String>,
    codes_path: &std::path::Path,
) -> anyhow::Result<String> {
    let value = value.context("--ref-text is required")?;
    let path = std::path::Path::new(&value);
    if path.is_file() {
        return Ok(std::fs::read_to_string(path)?.trim().to_string());
    }
    if value.is_empty() {
        let sibling = codes_path.with_extension("txt");
        if sibling.is_file() {
            return Ok(std::fs::read_to_string(sibling)?.trim().to_string());
        }
    }
    Ok(value)
}

fn print_help() {
    println!(
        "magichandy_neutts - MAGICHANDY_NEUTTS_STREAM_V1 persistent runner\n\
         \n\
         USAGE:\n\
         \tmagichandy_neutts --serve --codes PATH --ref-text TEXT [OPTIONS]\n\
         \n\
         REQUIRED:\n\
         \t--serve               Use the framed persistent worker protocol\n\
         \t--codes PATH          Pre-encoded .npy reference codes\n\
         \t--ref-text TEXT       Exact reference transcript or text-file path\n\
         \n\
         OPTIONS:\n\
         \t--backbone REPO       Hugging Face backbone repository\n\
         \t--gguf-file FILE      Exact cached GGUF filename\n\
         \t--espeak PATH         eSpeak NG executable (auto-detected by default)\n\
         \t--chunk N             Tokens per PCM chunk (default: 25)\n\
         \t--seed N|random       Sampler seed (default: 3; random disables PCM caching)\n\
         \t--phonemize TEXT      Print diagnostic IPA without loading models\n\
         \t--help                Show this help"
    );
}

#[cfg(test)]
mod tests {
    use super::*;

    fn reference_overlap_add(frames: &[Vec<f32>], stride: usize) -> Vec<f32> {
        let total_size = frames
            .iter()
            .enumerate()
            .map(|(index, frame)| index * stride + frame.len())
            .max()
            .unwrap_or(0);
        let mut output = vec![0.0f32; total_size];
        let mut weights = vec![0.0f32; total_size];
        for (frame_index, frame) in frames.iter().enumerate() {
            let offset = frame_index * stride;
            let denominator = (frame.len() + 1) as f32;
            for (sample_index, sample) in frame.iter().enumerate() {
                let position = (sample_index + 1) as f32 / denominator;
                let weight = 0.5 - (position - 0.5).abs();
                output[offset + sample_index] += weight * sample;
                weights[offset + sample_index] += weight;
            }
        }
        for (sample, weight) in output.iter_mut().zip(weights) {
            *sample /= weight;
        }
        output
    }

    #[test]
    fn incremental_overlap_matches_full_recomputation() {
        let frames = vec![
            vec![0.0, 0.1, 0.2, 0.3, 0.4, 0.5],
            vec![0.6, 0.5, 0.4, 0.3, 0.2, 0.1],
            vec![-0.2, -0.1, 0.0, 0.1, 0.2],
        ];
        let mut overlap = IncrementalOverlapAdd::new(4).unwrap();
        let mut actual = overlap.push(&frames[0]).unwrap();
        actual.extend(overlap.push(&frames[1]).unwrap());
        overlap.add(&frames[2]).unwrap();
        actual.extend(overlap.finish().unwrap());
        assert_eq!(actual, reference_overlap_add(&frames, 4));
    }

    #[test]
    fn pcm_cache_is_bounded_and_lru() {
        let mut cache = PcmCache::default();
        for index in 0..=PCM_CACHE_MAX_ENTRIES {
            cache.insert(index.to_string(), vec![index as u8; 2]);
        }
        assert!(cache.get("0").is_none());
        assert_eq!(cache.get("1"), Some(vec![1; 2]));
        assert!(cache.bytes <= PCM_CACHE_MAX_BYTES);
        assert!(cache.entries.len() <= PCM_CACHE_MAX_ENTRIES);

        cache.insert(
            "oversized".to_string(),
            vec![0; PCM_CACHE_MAX_ENTRY_BYTES + 1],
        );
        assert!(cache.get("oversized").is_none());
    }

    #[test]
    fn sampler_seed_supports_fixed_and_explicit_random_modes() {
        assert_eq!(parse_sampler_seed("3").unwrap(), Some(3));
        assert_eq!(parse_sampler_seed("random").unwrap(), None);
        assert!(parse_sampler_seed("-1").is_err());
    }
}
