<div align="center">

<h1>MagicHandy</h1>
<p><strong>Your devices. Your models. Your control.</strong></p>
<p>A free, open-source, local-first app for AI chat, hands-free motion, patterns, and video funscript playback with <a href="https://www.thehandy.com/">The Handy</a> and other supported devices.</p>
<p><a href="https://github.com/MagicHandy/MagicHandy/releases/tag/v0.1.0-alpha.74"><strong>Download for Windows</strong></a> &nbsp;·&nbsp; <a href="docs/getting-started.md">Getting started</a> &nbsp;·&nbsp; <a href="#documentation">Documentation</a> &nbsp;·&nbsp; <a href="#support-development">Support development</a></p>

</div>

> [!IMPORTANT]
> **Early alpha · v0.1.0-alpha.74 · Windows x64 · unsigned.** Expect rough
> edges. Read the [release notes](https://github.com/MagicHandy/MagicHandy/releases/tag/v0.1.0-alpha.74)
> for tested behavior and known limitations.

App data stays on your machine. Cloud models, device services, and remote
access are optional choices you enable explicitly.

## Highlights

| | What you can do |
|---|---|
| **AI chat & Autopilot** | Use managed llama.cpp, an existing Ollama installation, or a configured hosted/compatible provider. Choose Simple, Intermediate, or Advanced capabilities for each model. |
| **Live control** | Adjust speed, stroke, and direction immediately. Emergency Stop is always available, including with `Esc`. |
| **Devices** | Use Handy Cloud, browser Bluetooth, or Intiface. Supported vibrators and a DG-LAB Coyote 3.0 can run alone or alongside the Handy. See [Intiface](docs/intiface.md) and [e-stim](docs/estim.md). |
| **Patterns & video** | Explore the pattern library, build pattern programs, and play video with synchronized funscripts. |
| **Optional voice** | Add speech input and output through local or hosted providers. Models and voice modules are installed separately. |
| **Access your way** | Stay local, use your LAN, or explicitly set up Public access with HTTPS and accounts. See [HTTPS setup](docs/https-and-accounts.md). |

## Install on Windows

Download [**v0.1.0-alpha.74**](https://github.com/MagicHandy/MagicHandy/releases/tag/v0.1.0-alpha.74)
and choose the package that suits you:

| Package | Best for | Updating |
|---|---|---|
| **Setup EXE** | Standard installation, shortcuts, and Windows integration. | Install over the existing app to preserve settings and data. |
| **Portable ZIP** | Running without an installer or Windows integration. | Stop the app, then replace its application files. |

Verify the download against the release's `SHA256SUMS.txt`, launch MagicHandy,
and follow guided setup to choose your devices, model, and optional voice.
The [getting-started guide](docs/getting-started.md) covers installation and updates.

## Requirements

- **Platform:** Windows x64 is the primary supported platform. Linux and macOS
  are best-effort.
- **Device:** The Handy with firmware v4 and API v3 access, or another
  supported device through its documented transport.
- **AI:** Chat and Autopilot need a configured model provider; manual,
  pattern, and video controls do not. Models are not bundled.
- **Local models:** Easy setup checks your computer and recommends a tested,
  checksum-verified model that fits. The tested NVIDIA setup uses about 10 GB
  of graphics memory for the recommended 12B model, with a smaller E4B option
  for 6–8 GB cards. Other local endpoints can be configured in Settings.

## Build from source

<details>
<summary><strong>Windows PowerShell installation and developer commands</strong></summary>

Open PowerShell in the folder where MagicHandy should be installed and paste:

```powershell
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$bootstrap = Join-Path $env:TEMP 'MagicHandy-bootstrap.ps1'
Invoke-WebRequest -UseBasicParsing 'https://raw.githubusercontent.com/MagicHandy/MagicHandy/main/bootstrap.ps1' -OutFile $bootstrap
powershell.exe -NoProfile -ExecutionPolicy Bypass -File $bootstrap
```

The bootstrap installs missing build prerequisites. Device, model runtime,
and voice choices remain in the GUI. To update the same source installation:

```powershell
.\update.ps1
```

Developers with Go 1.26.10+ (Go 1.27 requires 1.27.3+) can run:

```powershell
go run ./cmd/magichandy
```

Open <http://127.0.0.1:49717>. See [Getting started](docs/getting-started.md)
for manual setup, deployment flags, and validation commands.

</details>

## Safety and privacy

- **Stop stays reachable.** Emergency Stop is mounted on every screen,
  including read-only tabs. All motion sources use one bounded engine and
  the limits you select.
- **Credentials stay private.** The Handy connection key is excluded from UI
  readback, logs, diagnostics, and exports.
- **Remote access is explicit.** Local only is the default. LAN mode rejects
  public peers; Internet access requires Public setup, HTTPS, an enabled
  backend account, and the port checklist.
- **Accounts share the app.** They gate entry but do not partition settings,
  chat, media, or libraries. A linked control profile does not impersonate
  another user or transfer the device-controller lease.
- **Hardware acceptance is ongoing.** Physical vibrator behavior, multi-Handy
  synchronization, and loaded COYOTE output remain unverified. Intiface
  Central may acknowledge Stop without delivering it to The Handy; use Native
  Bluetooth or Handy Cloud for its device Stop path.
- **Adults only.** MagicHandy controls intimate devices. Use it responsibly
  and at your own risk.

## Documentation

| Start here | Go deeper |
|---|---|
| [Getting started](docs/getting-started.md) | [Goals and guardrails](docs/goals-and-guardrails.md) |
| [Pattern library](docs/pattern-library.md) | [Motion and transport contract](docs/decisions/0002-motion-transport-contract.md) |
| [Video playback](docs/video-playback.md) | [LLM control surface](docs/llm-control-surface.md) |
| [HTTPS and accounts](docs/https-and-accounts.md) | [Account and control-context design](docs/account-gui-design.md) |
| [Versioning and releases](docs/versioning-and-releases.md) | [UI design guidelines](docs/ui-design-guidelines.md) |
| [Roadmap](IMPLEMENTATION_PLAN.md) | [Risk register](docs/risk-register.md) |

## Contributing

Bug reports, reproducible device feedback, documentation, and code contributions
are welcome. Start with [AGENTS.md](AGENTS.md) and the
[validation guide](docs/getting-started.md#validate-a-change).
Changes reach `main` through reviewed pull requests with green CI.

Chat motion, Autopilot, Freestyle, personas, memory, pattern programs, video
sync, voice providers, and model management are available for testing. Trusted
Windows signing and broader hardware acceptance remain open work.

## Support development

MagicHandy is free and open source, developed and maintained in spare time.
If you enjoy using it, a donation of any size helps cover AI development
costs, hardware testing, and replacements.

Scan a QR code or copy the full address below it.

<table align="center">
  <tr>
    <th align="center">Ethereum (ETH)</th>
    <th align="center">Bitcoin (BTC)</th>
  </tr>
  <tr>
    <td align="center"><img src="docs/assets/donations/ethereum-qr.svg" alt="Ethereum donation QR code" width="135" height="135"></td>
    <td align="center"><img src="docs/assets/donations/bitcoin-qr.svg" alt="Bitcoin donation QR code" width="135" height="135"></td>
  </tr>
</table>

**Ethereum (ETH)**

```text
0x1319841646b196F81283a1bf08d8a0256Cdd414B
```

**Bitcoin (BTC)**

```text
bc1pwqvmmzhdnmgp3px7l0ltsrrjk7hzlppnhhk6fm3e2l24xdvgpd7srm5zg6
```

## License

MagicHandy is licensed under the GNU General Public License v3.0 only.
See [LICENSE](LICENSE).
