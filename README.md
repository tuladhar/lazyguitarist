# guitarx-cli

**Lazy guitar effects for Linux.** Pick a song, plug in, play. No pedalboard, no patching, no fiddling with knobs.

I made this TUI on [Omarchy](https://omarchy.org) to help me play some of my favorite songs and solos without
setting up effects every time. Each song has a preset for each part (verse, solo, riff), and adding more songs is
just editing a JSON file.

![guitarfx screenshot](docs/screenshot.png)

## Demo

> 🎬 **Coming soon:** a demo video with voiceover.

<!-- Replace with the demo video link or embed, e.g. [![Demo](docs/demo-thumb.png)](https://youtu.be/...) -->

## Features

- **Song presets.** Each part of a song gets its own effect chain, and `1`–`9` switches parts instantly, e.g.
  clean verse → solo. All presets are evened out to the same loudness, so switching never blasts your ears.
- **Tuner.** Standard tuning (E A D G B E). It tells you which string you're on and whether to **tighten** or
  **loosen** it, and by how much. The amp is muted while you tune, and 50/60 Hz cable hum is ignored.
- **Recording.** Press `r` to record exactly what you hear, and `r` again to stop. A blinking
  `● REC 0:12` shows while recording. Files are clean 48 kHz / 32-bit WAVs with click-free fades, saved in
  `~/Music/guitarfx/`.
- **Low latency.** It runs PipeWire at a 128-sample buffer (2.7 ms), so playing feels instant through wired
  headphones. Quitting puts the buffer setting back.
- **Zero setup.** It finds your USB guitar dongle, wires everything with PipeWire and starts the
  [Guitarix](https://guitarix.org) engine in the background with no window. You just see the TUI.

### Included presets

| # | Song | Part | Chain |
|---|---|---|---|
| 1 | Pink Floyd - *Coming Back to Life* | Intro & verses | compressor › clean amp › Twin tonestack › 4x12 › 450 ms echo › hall reverb |
| 2 | | Solo | gate › compressor › TS9 › pushed amp › 4x12 › 450 ms echo › reverb |
| 3 | Black Sabbath - *N.I.B.* | Main riff | gate › Rangemaster treble booster › high-gain amp (JCM-800 tonestack, mids up) › 4x12 › room |
| 4 | | Solo | same + auto-wah |

These are by-ear approximations built from Guitarix's amp and pedal models, not copies of the original rigs.

## How it works

```
Fender ──► USB dongle (mic in) ──► Guitarix (headless) ──► headphones (wired)
                                        ▲
                     guitarfx TUI ──────┘  JSON-RPC: presets, tuner, volume
                     PipeWire (pw-link)    wiring + 128-sample buffer
                     pw-record             recording
```

## Prerequisites

| What | Arch / Omarchy package | Why |
|---|---|---|
| Guitarix | `guitarix` | the amp/effects engine |
| PipeWire + JACK support | `pipewire`, `pipewire-jack` | audio, and lets Guitarix run as a JACK app (no `jackd` needed) |
| PipeWire tools | `pipewire-audio` (`pw-record`), `pipewire-pulse` + `libpulse` (`pactl`) | recording, device list, input gain |
| WirePlumber | `wireplumber` | PipeWire session manager (Omarchy default) |
| Go 1.24+ | `go` (or via `mise`) | to build it |

```bash
sudo pacman -S --needed guitarix pipewire pipewire-jack pipewire-audio pipewire-pulse wireplumber go
```

Omarchy already ships PipeWire, WirePlumber and pipewire-pulse, so on Omarchy it's mostly `guitarix` and `go`.
You don't need `jackd`, a `realtime` group or any extra drivers: PipeWire gets realtime priority through RTKit, and
USB audio dongles use the kernel's standard `snd-usb-audio` driver.

## Install & run

```bash
git clone https://github.com/tuladhar/guitarx-cli.git
cd guitarx-cli
go build -o guitarfx .
ln -s "$PWD/guitarfx" ~/.local/bin/guitarfx   # optional: run it from anywhere

guitarfx            # preset menu
guitarfx --tuner    # open straight into the tuner
guitarfx --check    # list audio devices and what guitarfx would use
```

### Keys

| Key | Action |
|---|---|
| `↑`/`↓` + `Enter`, or `1`–`9` | load a preset |
| `t` | tuner (any key goes back) |
| `r` | start / stop recording |
| `b` | bypass all effects (dry guitar) |
| `-` / `+` | output volume (starts at -12 dB) |
| `[` / `]` | input gain on the dongle |
| `i` / `o` | switch input / output device |
| `l` | buffer size 64 / 128 / 256 (lower = less latency, more risk of crackles) |
| `q` | quit |

## My setup (known good)

| | |
|---|---|
| OS | Omarchy 4.5.0 (Arch Linux), kernel `7.2.15-1-omarchy` |
| Audio | PipeWire 1.6.9, WirePlumber 0.5.18, pipewire-jack 1.6.9, 48 kHz |
| Engine | Guitarix 0.47.0 |
| Guitar | Fender electric |
| USB dongle | Unitek Y-247A USB audio adapter (C-Media chip, `snd-usb-audio` driver). The guitar goes into its **mic input** (6.35 mm → 3.5 mm adapter). Mono, 48 kHz. |
| Headphones | Sony, wired, into the **laptop's 3.5 mm headphone jack** (Ryzen HD Audio, `snd_hda_intel`). Wired on purpose: Bluetooth adds far too much delay for playing live. |
| Terminal | Foot |

### Preferred settings

- **Buffer:** 128 samples @ 48 kHz (`l` to change). guitarfx sets it with
  `pw-metadata -n settings 0 clock.force-quantum 128` while it runs and restores it on quit.
- **Output:** the wired headphone jack (`o` to pick). Plugging the headphones into the dongle itself saves another
  ~1–2 ms, because input and output then share one clock.
- **Input gain:** start at 100% and lower it with `[` if clean tones distort or high-gain sounds fizzy. Mic inputs
  are hot.
- **Volume:** start at -12 dB with the headphones off your ears, then bring it up.
- **Firewall:** keep `ufw` on (Omarchy's default). Guitarix's control port has no password and listens on all
  interfaces while guitarfx runs, and ufw blocks it from the network.
- **Avoid 16 kHz "mic" dongles:** some USB receivers only do 16 kHz, which sounds like a phone call.
  `guitarfx --check` shows each device's sample rate. You want 44.1 or 48 kHz.

## Adding songs

Presets live in `presets.json`. guitarfx reads `~/.config/guitarfx/presets.json` first, then the one next to the
binary, then the copy built into the binary. Each part looks like this:

```json
{
  "name": "Solo (singing lead)",
  "description": "Shown in the Now playing panel.",
  "level_db": 8.8,
  "amp": { "model": "pre 12AT7/ push pull 6V6", "pregain": 0, "drive": -4 },
  "fx": {
    "compressor": { "ratio": 5, "threshold": -28 },
    "ts9sim":     { "drive": 0.45, "tone": 550, "level": -10 },
    "cab":        { "select": "4x12" },
    "echo":       { "bpm": 133, "percent": 32 },
    "zita_rev1":  { "output.dry_wet_mix": -0.45 }
  }
}
```

- **`amp`**: `model` is a Guitarix tube model (`12ax7`, `12AU7`, `6V6`, `pre 12ax7/ push-pull 6V6`, …);
  `pregain` and `drive` are -20..20 dB.
- **`fx`**: effects to turn on, with Guitarix parameter values. Anything not listed is off. Supported:
  `noise_gate`, `crybaby` (wah), `compressor`, `rangem` (treble booster), `ts9sim` (Tube Screamer), `muff` (Big Muff)
  before the amp; `amp.tonestack`, `cab`, `echo` after it; `zita_rev1` reverb.
- **`echo.bpm`** sets the echo time: one repeat every 60000/bpm ms (133 ≈ 451 ms).
- **`level_db`** evens out loudness between parts (-40..40 dB). Distorted parts need much less than clean ones.

Typos are safe: unknown effects or parameters are skipped and shown in red instead of being sent to Guitarix.

## Files

| Path | What |
|---|---|
| `~/.config/guitarfx/config.json` | chosen devices, volume, buffer, last preset |
| `~/Music/guitarfx/` | recordings |
| `~/.cache/guitarfx/guitarix.log` | Guitarix engine log |
| `~/.config/guitarix/guitarfx_rc` | Guitarix's state for this instance (your normal Guitarix settings are untouched) |

If guitarfx is ever killed with `kill -9`, restore PipeWire's buffer with
`pw-metadata -n settings 0 clock.force-quantum 0`.

## Built with

[Guitarix](https://guitarix.org) · [PipeWire](https://pipewire.org) · [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss) by [Charm](https://charm.land)

## License

[MIT](LICENSE)

---

<p align="center">made with ♥ by Puru · for hobby guitarists</p>
