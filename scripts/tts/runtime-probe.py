import importlib
import sys


def main() -> None:
    if len(sys.argv) != 3:
        raise RuntimeError("usage: runtime-probe.py <module> <device>")

    module_name, device = sys.argv[1:3]
    required = [
        "torch",
        "numpy",
        "soundfile",
        "fastapi",
        "uvicorn",
        "huggingface_hub",
    ]
    if module_name == "faster-qwen3-tts":
        required.extend(["faster_qwen3_tts", "qwen_tts"])
    else:
        required.extend(
            [
                "chatterbox.tts",
                "chatterbox.tts_turbo",
                "s3tokenizer",
                "onnx",
                "google.protobuf",
                "librosa",
                "torchaudio",
                "perth",
                "pyloudnorm",
            ]
        )

    for dependency in required:
        importlib.import_module(dependency)

    import torch

    if device == "cuda" and not torch.cuda.is_available():
        raise RuntimeError(
            "the installed PyTorch CUDA runtime cannot use this NVIDIA driver; "
            "update the driver or choose a CPU-capable voice module"
        )
    backend = torch.cuda.get_device_name(0) if torch.cuda.is_available() else "CPU"
    print(f"Verified Python voice runtime ({backend}).")


if __name__ == "__main__":
    main()
