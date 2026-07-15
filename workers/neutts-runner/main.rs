// MagicHandy persistent NeuTTS runner.
// Copyright (C) 2026 MagicHandy contributors
// SPDX-License-Identifier: GPL-3.0-only

use std::collections::HashSet;
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

struct Options {
    codes_path: PathBuf,
    ref_text: String,
    backbone: String,
    gguf_file: Option<String>,
    chunk_size: usize,
    espeak_path: PathBuf,
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
    let tts = neutts::download::load_from_hub_cb(
        &options.backbone,
        options.gguf_file.as_deref(),
        |_| {},
    )?;
    let codec_backend = tts.codec.backend_name().to_string();
    let ref_codes = tts
        .load_ref_codes(&options.codes_path)
        .with_context(|| format!("load reference codes from {}", options.codes_path.display()))?;
    let ref_phones = phonemize(&options.espeak_path, &options.ref_text)
        .context("phonemize reference transcript")?;

    let canceled = Arc::new(Mutex::new(HashSet::<String>::new()));
    let work = spawn_input_reader(Arc::clone(&canceled));
    let stdout = io::stdout();
    let mut out = io::BufWriter::new(stdout.lock());
    let ready = serde_json::json!({
        "protocol": RUNNER_PROTOCOL,
        "codec": codec_backend,
        "load_ms": load_started.elapsed().as_millis(),
    });
    write_frame(&mut out, FRAME_READY, &serde_json::to_vec(&ready)?)?;
    eprintln!(
        "[magichandy-neutts] ready: codec={}, load={:.2}s",
        tts.codec.backend_name(),
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
                let mut audio_frames = Vec::<Vec<f32>>::new();
                let mut emitted_samples = 0usize;
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
                        audio_frames.push(frame);
                        let mixed =
                            linear_overlap_add(&audio_frames, options.chunk_size * HOP_LENGTH)?;
                        let next_end = audio_frames.len() * options.chunk_size * HOP_LENGTH;
                        let next_end = next_end.min(mixed.len());
                        write_audio_frame(&mut out, &mixed[emitted_samples..next_end])?;
                        samples += next_end - emitted_samples;
                        emitted_samples = next_end;
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
                        audio_frames.push(frame);
                        let mixed =
                            linear_overlap_add(&audio_frames, options.chunk_size * HOP_LENGTH)?;
                        write_audio_frame(&mut out, &mixed[emitted_samples..])?;
                        samples += mixed.len() - emitted_samples;
                    }
                    Ok(())
                });

                let was_canceled = is_canceled(&canceled, &id);
                clear_canceled(&canceled, &id);
                match synthesis {
                    _ if was_canceled => write_frame(&mut out, FRAME_CANCELED, &[])?,
                    Ok(()) if samples > 0 => {
                        write_frame(&mut out, FRAME_DONE, &[])?;
                        eprintln!(
                            "[magichandy-neutts] request {}: {:.2}s, {:.2}s audio",
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

fn linear_overlap_add(frames: &[Vec<f32>], stride: usize) -> anyhow::Result<Vec<f32>> {
    anyhow::ensure!(!frames.is_empty(), "overlap-add requires an audio frame");
    anyhow::ensure!(stride > 0, "overlap-add stride must be positive");
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
        anyhow::ensure!(weight > 0.0, "overlap-add produced an uncovered sample");
        *sample /= weight;
    }
    Ok(output)
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

fn write_audio_frame(out: &mut impl io::Write, samples: &[f32]) -> anyhow::Result<()> {
    let mut pcm = vec![0u8; samples.len() * 2];
    for (index, &sample) in samples.iter().enumerate() {
        let bytes = ((sample.clamp(-1.0, 1.0) * i16::MAX as f32) as i16).to_le_bytes();
        pcm[index * 2] = bytes[0];
        pcm[index * 2 + 1] = bytes[1];
    }
    write_frame(out, FRAME_AUDIO, &pcm)
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
    })))
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
         \t--phonemize TEXT      Print diagnostic IPA without loading models\n\
         \t--help                Show this help"
    );
}
