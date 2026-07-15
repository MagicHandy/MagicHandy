use std::{
    env,
    error::Error,
    fs,
    io::Write,
    path::{Path, PathBuf},
    process,
};

use audioadapter_buffers::direct::InterleavedSlice;
use ort::{session::Session, value::Tensor};
use rubato::{Fft, FixedSync, Resampler};

const TARGET_SAMPLE_RATE: u32 = 16_000;
const MIN_SAMPLE_RATE: u32 = 16_000;
const MAX_SAMPLE_RATE: u32 = 48_000;
const MIN_DURATION_SECONDS: f64 = 1.0;
const MAX_DURATION_SECONDS: f64 = 30.0;
const CODE_HOP_SAMPLES: usize = 320;

struct Arguments {
    model: PathBuf,
    input: PathBuf,
    output: PathBuf,
}

struct Audio {
    samples: Vec<f32>,
    source_sample_rate: u32,
    duration_seconds: f64,
}

fn main() {
    if let Err(error) = run() {
        eprintln!("NeuCodec reference encoding failed: {error}");
        process::exit(1);
    }
}

fn run() -> Result<(), Box<dyn Error>> {
    let arguments = parse_arguments()?;
    let audio = read_wav(&arguments.input)?;
    let mut samples = if audio.source_sample_rate == TARGET_SAMPLE_RATE {
        audio.samples
    } else {
        resample(audio.samples, audio.source_sample_rate)?
    };

    // Match DistillNeuCodec's published preprocessing exactly, including one
    // full zero frame when the input already lands on a hop boundary.
    let padding = CODE_HOP_SAMPLES - samples.len() % CODE_HOP_SAMPLES;
    samples.resize(samples.len() + padding, 0.0);

    let mut session = Session::builder()?.commit_from_file(&arguments.model)?;
    let input = Tensor::from_array(([1usize, 1, samples.len()], samples.into_boxed_slice()))?;
    let outputs = session.run(ort::inputs!["audio" => input])?;
    let (_, codes) = outputs["codes"].try_extract_tensor::<i32>()?;
    validate_codes(codes)?;
    write_npy(&arguments.output, codes)?;
    println!(
        "{{\"token_count\":{},\"source_sample_rate\":{},\"duration_seconds\":{:.3}}}",
        codes.len(),
        audio.source_sample_rate,
        audio.duration_seconds,
    );
    Ok(())
}

fn parse_arguments() -> Result<Arguments, Box<dyn Error>> {
    let mut model = None;
    let mut input = None;
    let mut output = None;
    let mut arguments = env::args().skip(1);
    while let Some(argument) = arguments.next() {
        match argument.as_str() {
            "--help" | "-h" => {
                print_help();
                process::exit(0);
            }
            "--version" | "-V" => {
                println!("magichandy-neucodec-encoder {}", env!("CARGO_PKG_VERSION"));
                process::exit(0);
            }
            "--model" => model = Some(required_value("--model", arguments.next())?),
            "--input" => input = Some(required_value("--input", arguments.next())?),
            "--output" => output = Some(required_value("--output", arguments.next())?),
            _ => return Err(format!("unknown argument {argument:?}; run with --help").into()),
        }
    }
    Ok(Arguments {
        model: model.ok_or("--model is required")?,
        input: input.ok_or("--input is required")?,
        output: output.ok_or("--output is required")?,
    })
}

fn required_value(name: &str, value: Option<String>) -> Result<PathBuf, Box<dyn Error>> {
    let value = value.ok_or_else(|| format!("{name} requires a path"))?;
    if value.trim().is_empty() {
        return Err(format!("{name} requires a non-empty path").into());
    }
    Ok(PathBuf::from(value))
}

fn print_help() {
    println!("MagicHandy NeuCodec reference encoder");
    println!();
    println!("Usage:");
    println!(
        "  magichandy-neucodec-encoder --model MODEL.onnx --input REFERENCE.wav --output CODES.npy"
    );
    println!();
    println!("The WAV must contain 1-30 seconds of audio sampled at 16-48 kHz.");
    println!("Stereo and multichannel inputs are downmixed to mono before encoding.");
}

fn read_wav(path: &Path) -> Result<Audio, Box<dyn Error>> {
    let mut reader = hound::WavReader::open(path)?;
    let spec = reader.spec();
    if spec.channels == 0 || spec.channels > 8 {
        return Err(format!("WAV channel count {} is unsupported", spec.channels).into());
    }
    if !(MIN_SAMPLE_RATE..=MAX_SAMPLE_RATE).contains(&spec.sample_rate) {
        return Err(format!(
            "WAV sample rate must be between {MIN_SAMPLE_RATE} and {MAX_SAMPLE_RATE} Hz; got {} Hz",
            spec.sample_rate,
        )
        .into());
    }

    let interleaved = match spec.sample_format {
        hound::SampleFormat::Float => reader.samples::<f32>().collect::<Result<Vec<_>, _>>()?,
        hound::SampleFormat::Int if spec.bits_per_sample <= 8 => reader
            .samples::<i8>()
            .map(|sample| sample.map(|value| value as f32 / 128.0))
            .collect::<Result<Vec<_>, _>>()?,
        hound::SampleFormat::Int if spec.bits_per_sample <= 16 => reader
            .samples::<i16>()
            .map(|sample| sample.map(|value| value as f32 / 32_768.0))
            .collect::<Result<Vec<_>, _>>()?,
        hound::SampleFormat::Int if spec.bits_per_sample <= 32 => {
            let scale = (1u64 << (spec.bits_per_sample - 1)) as f32;
            reader
                .samples::<i32>()
                .map(|sample| sample.map(|value| value as f32 / scale))
                .collect::<Result<Vec<_>, _>>()?
        }
        _ => return Err(format!("WAV bit depth {} is unsupported", spec.bits_per_sample).into()),
    };
    let channels = spec.channels as usize;
    if interleaved.len() % channels != 0 {
        return Err("WAV contains an incomplete audio frame".into());
    }
    let frames = interleaved.len() / channels;
    let duration_seconds = frames as f64 / spec.sample_rate as f64;
    if !(MIN_DURATION_SECONDS..=MAX_DURATION_SECONDS).contains(&duration_seconds) {
        return Err(format!(
            "reference audio must be between {MIN_DURATION_SECONDS:.0} and {MAX_DURATION_SECONDS:.0} seconds; got {duration_seconds:.2} seconds",
        ).into());
    }

    let mut mono = Vec::with_capacity(frames);
    for frame in interleaved.chunks_exact(channels) {
        let sample = frame.iter().copied().sum::<f32>() / channels as f32;
        if !sample.is_finite() {
            return Err("WAV contains a non-finite sample".into());
        }
        mono.push(sample.clamp(-1.0, 1.0));
    }
    Ok(Audio {
        samples: mono,
        source_sample_rate: spec.sample_rate,
        duration_seconds,
    })
}

fn resample(samples: Vec<f32>, source_sample_rate: u32) -> Result<Vec<f32>, Box<dyn Error>> {
    let input_frames = samples.len();
    let input = InterleavedSlice::new(&samples, 1, input_frames)?;
    let mut resampler = Fft::<f32>::new(
        source_sample_rate as usize,
        TARGET_SAMPLE_RATE as usize,
        1024,
        1,
        FixedSync::Both,
    )?;
    Ok(resampler
        .process_all(&input, input_frames, None)?
        .take_data())
}

fn validate_codes(codes: &[i32]) -> Result<(), Box<dyn Error>> {
    if codes.is_empty() {
        return Err("encoder returned no reference codes".into());
    }
    if codes.iter().any(|code| !(0..=65_535).contains(code)) {
        return Err("encoder returned a code outside the NeuCodec range".into());
    }
    Ok(())
}

fn write_npy(path: &Path, codes: &[i32]) -> Result<(), Box<dyn Error>> {
    let mut header = format!(
        "{{'descr': '<i4', 'fortran_order': False, 'shape': ({},), }}",
        codes.len()
    );
    let base_len = 10 + header.len() + 1;
    let padding = (64 - base_len % 64) % 64;
    header.push_str(&" ".repeat(padding));
    header.push('\n');
    if header.len() > u16::MAX as usize {
        return Err("NumPy header is too large".into());
    }
    let mut file = fs::File::create(path)?;
    file.write_all(b"\x93NUMPY")?;
    file.write_all(&[1, 0])?;
    file.write_all(&(header.len() as u16).to_le_bytes())?;
    file.write_all(header.as_bytes())?;
    for code in codes {
        file.write_all(&code.to_le_bytes())?;
    }
    file.sync_all()?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temporary_path(extension: &str) -> PathBuf {
        let nonce = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system clock must be after the Unix epoch")
            .as_nanos();
        env::temp_dir().join(format!(
            "magichandy-neucodec-{}-{nonce}.{extension}",
            process::id()
        ))
    }

    #[test]
    fn writes_one_dimensional_little_endian_numpy_codes() {
        let path = temporary_path("npy");
        write_npy(&path, &[0, 32_768, 65_535]).expect("write NPY");
        let data = fs::read(&path).expect("read NPY");
        let _ = fs::remove_file(path);

        assert_eq!(&data[..8], b"\x93NUMPY\x01\x00");
        let header_length = u16::from_le_bytes([data[8], data[9]]) as usize;
        let header = std::str::from_utf8(&data[10..10 + header_length]).expect("ASCII header");
        assert!(header.contains("'descr': '<i4'"));
        assert!(header.contains("'shape': (3,)"));
        assert_eq!((10 + header_length) % 64, 0);
        let values = data[10 + header_length..]
            .chunks_exact(4)
            .map(|bytes| i32::from_le_bytes(bytes.try_into().expect("four bytes")))
            .collect::<Vec<_>>();
        assert_eq!(values, [0, 32_768, 65_535]);
    }

    #[test]
    fn reads_and_downmixes_stereo_pcm_wav() {
        let path = temporary_path("wav");
        let specification = hound::WavSpec {
            channels: 2,
            sample_rate: TARGET_SAMPLE_RATE,
            bits_per_sample: 16,
            sample_format: hound::SampleFormat::Int,
        };
        let mut writer = hound::WavWriter::create(&path, specification).expect("create WAV");
        for _ in 0..TARGET_SAMPLE_RATE {
            writer.write_sample(16_384_i16).expect("left sample");
            writer.write_sample(-16_384_i16).expect("right sample");
        }
        writer.finalize().expect("finalize WAV");

        let audio = read_wav(&path).expect("read WAV");
        let _ = fs::remove_file(path);
        assert_eq!(audio.source_sample_rate, TARGET_SAMPLE_RATE);
        assert_eq!(audio.samples.len(), TARGET_SAMPLE_RATE as usize);
        assert!((audio.duration_seconds - 1.0).abs() < f64::EPSILON);
        assert!(
            audio
                .samples
                .iter()
                .all(|sample| sample.abs() < f32::EPSILON)
        );
    }

    #[test]
    fn rejects_codes_outside_neucodec_range() {
        assert!(validate_codes(&[]).is_err());
        assert!(validate_codes(&[-1]).is_err());
        assert!(validate_codes(&[65_536]).is_err());
        assert!(validate_codes(&[0, 65_535]).is_ok());
    }
}
