// MagicHandy persistent NeuTTS runner.
// Copyright (C) 2026 MagicHandy contributors
// SPDX-License-Identifier: GPL-3.0-only

use std::collections::HashSet;
use std::io::{self, BufRead};
use std::path::PathBuf;
use std::sync::{mpsc, Arc, Mutex};
use std::time::Instant;

use anyhow::Context as _;
use serde::Deserialize;

const FRAME_MAGIC: &[u8; 4] = b"MHTS";
const FRAME_READY: u8 = 1;
const FRAME_AUDIO: u8 = 2;
const FRAME_DONE: u8 = 3;
const FRAME_ERROR: u8 = 4;
const FRAME_CANCELED: u8 = 5;
const RUNNER_PROTOCOL: u32 = 1;

struct Options {
    codes_path: PathBuf,
    ref_text: String,
    backbone: String,
    gguf_file: Option<String>,
    chunk_size: usize,
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
    let Some(options) = parse_args()? else {
        return Ok(());
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
    let ref_phones = neutts::phonemize::phonemize(&options.ref_text, "en-us")
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

                let input_phones = match neutts::phonemize::phonemize(&text, "en-us") {
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
                let mut pending = Vec::<i32>::with_capacity(options.chunk_size + 8);
                let mut samples = 0usize;

                let synthesis = backbone.generate_streaming(&prompt, 2048, |piece| {
                    if is_canceled(&canceled, &id) {
                        anyhow::bail!("synthesis canceled");
                    }
                    let ids = neutts::tokens::extract_ids(piece);
                    if ids.is_empty() {
                        return Ok(());
                    }
                    pending.extend_from_slice(&ids);
                    if pending.len() < options.chunk_size {
                        return Ok(());
                    }
                    let audio = codec.decode(&pending).context("NeuCodec decode failed")?;
                    samples += audio.len();
                    write_audio_frame(&mut out, &audio)?;
                    pending.clear();
                    Ok(())
                });

                let synthesis = synthesis.and_then(|_| {
                    if is_canceled(&canceled, &id) {
                        anyhow::bail!("synthesis canceled");
                    }
                    if !pending.is_empty() {
                        let audio = codec
                            .decode(&pending)
                            .context("NeuCodec tail decode failed")?;
                        samples += audio.len();
                        write_audio_frame(&mut out, &audio)?;
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

fn parse_args() -> anyhow::Result<Option<Options>> {
    let mut serve = false;
    let mut codes_path = None;
    let mut ref_text = None;
    let mut backbone = "neuphonic/neutts-air-q4-gguf".to_string();
    let mut gguf_file = None;
    let mut chunk_size = 25usize;
    let mut args = std::env::args().skip(1);
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--serve" => serve = true,
            "--codes" | "-c" => codes_path = args.next().map(PathBuf::from),
            "--ref-text" | "-r" => ref_text = args.next(),
            "--backbone" | "-b" => backbone = args.next().unwrap_or(backbone),
            "--gguf-file" | "-g" => gguf_file = args.next(),
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
    anyhow::ensure!(serve, "--serve is required; run with --help");
    anyhow::ensure!(chunk_size > 0, "--chunk must be greater than zero");
    let codes_path = codes_path.context("--codes is required")?;
    let ref_text = resolve_reference_text(ref_text, &codes_path)?;
    anyhow::ensure!(!ref_text.trim().is_empty(), "reference transcript is empty");
    Ok(Some(Options {
        codes_path,
        ref_text,
        backbone,
        gguf_file,
        chunk_size,
    }))
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
         \t--chunk N             Tokens per PCM chunk (default: 25)\n\
         \t--help                Show this help"
    );
}
