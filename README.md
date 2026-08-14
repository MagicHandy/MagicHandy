# MagicHandy

MagicHandy is a free, open-source, local-first app for controlling
[The Handy](https://www.thehandy.com/) with a customizable AI chatbot, hands-free modes,
pattern library, and video funscript playback. App data stays on your machine, with
cloud services available only when you enable them.

[Download](https://github.com/MagicHandy/MagicHandy/releases/tag/v0.1.0-alpha.22)
| [Getting Started](docs/getting-started.md)
| [Roadmap](IMPLEMENTATION_PLAN.md)
| [Contributing](#contributing)

> [!IMPORTANT]
> MagicHandy is still in early development (alpha). The current
> Windows build is `v0.1.0-alpha.22` and remains unsigned.

## Highlights

- Local LLM chat through managed llama.cpp or an existing Ollama installation.
- Immediate speed, stroke, and direction controls with safe `Esc` Stop.
- Handy Cloud, browser Bluetooth, and Intiface support.
- Optional local speech input and output modules.

## Install On Windows

Download the installer package from the
[latest release](https://github.com/MagicHandy/MagicHandy/releases/tag/v0.1.0-alpha.22):

- **Setup EXE:** standard installation, shortcuts, and Windows integration.
- **Portable ZIP:** no installer or Windows integration.

## Build From Source

Open PowerShell in the folder where MagicHandy should be installed and paste:

```powershell
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$bootstrap = Join-Path $env:TEMP 'MagicHandy-bootstrap.ps1'
Invoke-WebRequest -UseBasicParsing 'https://raw.githubusercontent.com/MagicHandy/MagicHandy/main/bootstrap.ps1' -OutFile $bootstrap
powershell.exe -NoProfile -ExecutionPolicy Bypass -File $bootstrap
```

The bootstrap installs missing build prerequisites. Device, model runtime, and
voice choices remain in the GUI. To update the same source installation later:

```powershell
.\update.ps1
```

Developers with Go 1.25+ can run `go run ./cmd/magichandy` and open
<http://127.0.0.1:49717>. See the
[Getting Started guide](docs/getting-started.md) for manual setup, deployment
flags, and validation commands.

## Requirements

- Windows x64 is the primary supported platform. Linux and macOS are
  best-effort.
- A Handy with firmware v4 and API v3 access, or a supported linear actuator
  connected through Intiface Central.
- A local LLM is needed for Chat and Autopilot, but not for manual, pattern, or
  video control. Models are not bundled.

## Safety And Privacy

- Emergency Stop remains available on every screen, including read-only tabs.
- All motion sources use the same bounded motion engine and selected limits.
- The Handy connection key is private and excluded from UI readback, logs,
  diagnostics, and exports.
- MagicHandy is for adults controlling an intimate device. Use it responsibly
  and at your own risk.

## Project Status

Chat motion, Autopilot, Freestyle, personas, memory, pattern programs, video
sync, voice providers, and model management are available for testing. Trusted
Windows signing and broader release acceptance remain open work. See the
[implementation plan](IMPLEMENTATION_PLAN.md) and
[risk register](docs/risk-register.md) for current status.

## Contributing

Changes use feature branches and merge into `main` through reviewed pull
requests with green CI. Start with [AGENTS.md](AGENTS.md), then use the
[validation guide](docs/getting-started.md#validate-a-change).

## Documentation

- [Getting Started](docs/getting-started.md)
- [Goals and guardrails](docs/goals-and-guardrails.md)
- [Motion and transport contract](docs/decisions/0002-motion-transport-contract.md)
- [Pattern library](docs/pattern-library.md)
- [Video playback](docs/video-playback.md)
- [LLM control surface](docs/llm-control-surface.md)
- [UI design guidelines](docs/ui-design-guidelines.md)
- [Versioning and releases](docs/versioning-and-releases.md)

## License

MagicHandy is licensed under the GNU General Public License v3.0 only. See
[LICENSE](LICENSE).
