---
name: preset
description: Add a guitar tone preset for a song to lazyguitarist's presets.json. Use when the user runs /preset <song name> or asks to add, create or make a preset or tone for a song, solo or riff (e.g. "/preset Comfortably Numb", "add a preset for Iron Man", "make a tone for the Hotel California solo").
argument-hint: <song name> [artist] [part, e.g. "solo"]
---

# Add a song preset to lazyguitarist

The user wants a preset for: **$ARGUMENTS**

A preset is one song with one or more **parts** (riff, verse, solo…), each with its own effect chain, in
`presets.json` at the repo root. Work through these steps in order.

## 1. Pin down the song

- Work out the artist and song from `$ARGUMENTS`. If it's ambiguous (several famous songs with that name),
  ask the user which one before going further.
- If `presets.json` already has this song, add or update parts on the existing entry instead of
  duplicating it. Otherwise append a new song object at the end of `songs`.
- If the user named a part ("solo", "intro riff"), make that part. Otherwise make the 1–3 parts a hobby
  guitarist would actually want (usually the main riff or rhythm sound plus the solo; for clean/ambient songs,
  clean verses plus the lead).

## 2. Research the tone

Search the web for how the guitarist got that sound **on that recording**: amp, drive or fuzz pedals,
wah, compression, delay time (or tempo and note value), reverb, and how clean or dirty each part is.
Good sources are rig rundowns, Equipboard, interviews, and gear forums for the band. Then note:

- **Tempo:** you need it if the part has a tempo-synced delay.
- **Tuning:** if it isn't standard E (e.g. Sabbath often tuned down, drop D songs), mention it in the
  `description`, since lazyguitarist's tuner shows standard tuning.

### If the user gives an audio file of the song

Learn from the recording itself; it beats web research, especially for smaller bands. Convert it to mono WAV
with `ffmpeg` in the scratchpad, and analyze it with Python (`librosa`, in a scratchpad venv, run with `-I`):

- **Tempo** (`librosa.beat.beat_track`), and the delay time from the autocorrelation of the onset envelope in
  each section: repeats a beat apart mean a quarter-note echo; other spacings give `bpm` = 60000 / ms.
- **Tuning and key:** `librosa.estimate_tuning` (near 0 means A440) and the mean chroma.
- **Sections:** RMS, spectral centroid and flatness every 5 s (and every 1 s for the intro) show where the
  intro, verses, choruses and solo are. Quiet, no drums and a low centroid means a soft clean part; high
  flatness means distortion.
- **Tone per section:** run harmonic/percussive separation (`librosa.effects.hpss`) and compare band
  energies (60 Hz–11 kHz in octaves) on the harmonic part. Dark sections get less treble, and a strong
  400 Hz–1.6 kHz bump means a mid-forward lead (TS9 and `Middle` up).

Make one part per distinct section, say in each `description` which part of the record it was matched to
(e.g. "Matched to the verse at 0:25"), and tell the user what you measured.

## 3. Map the rig onto lazyguitarist's building blocks

Only these effects exist (in signal order). Run `./lazyguitarist --params units` to confirm:

| Block | Use for |
|---|---|
| `abgate` | noise gate for any driven part: `{ "threshold": -22, "attack": 1, "hold": 60, "decay": 300, "gaterange": -90 }` for high gain (pregain ≥ 8); threshold -24 and decay 400 for crunch. The user's Strat hums at about -26 dB peak at the input, so the threshold must stay above that. Guitarix's `abgate` can't gate it and is no longer available |
| `crybaby` | wah: `"autowah": "auto"` for an envelope wah, or `"manual"` with a fixed `wah` position for a cocked-wah tone |
| `compressor` | clean sustain, country/funk squash, Gilmour-style leads |
| `rangem` | Rangemaster-style treble booster (Iommi, early Brian May, Clapton Beano) |
| `ts9sim` | Tube Screamer / mild overdrive / mid-hump lead boost |
| `muff` | Big Muff fuzz (Gilmour solos, Billy Corgan, J Mascis) |
| `amp` (part's `amp` block) | `model` (tube type), `pregain` and `drive` (-20..20 dB; clean ≈ -6/-8, crunch ≈ 0/0, high gain ≈ 12/4) |
| `amp.tonestack` | amp voicing via `select` (e.g. "Twin Reverb", "JCM-800", "AC-30", "Mesa Boogie", "Bassman") plus `Bass`/`Middle`/`Treble` 0..1 |
| `cab` | speaker cab via `select` (e.g. "4x12", "2x12", "1x12", "AC-30", "Twin") |
| `echo` | delay: `bpm` sets the time (repeat every 60000/bpm ms) and `percent` is the feedback |
| `zita_rev1` | reverb: `output.dry_wet_mix` from -1 (dry) to 1 (wet); ~-0.85 is a small room, ~-0.35 is a big hall; plus `decay_times.mid_rt60` |

**Echo time from tempo:** quarter-note delay → `bpm` = song tempo. Dotted eighth (the Edge / Gilmour
"Run Like Hell" style) → `bpm` = tempo / 0.75. Eighth → `bpm` = tempo × 2. A delay given in ms → `bpm` = 60000 / ms.

**Always check real setting names and options** before writing them:

```bash
go build -o lazyguitarist .
./lazyguitarist --params amp          # amp models, gain ranges
./lazyguitarist --params cab          # cab options
./lazyguitarist --params ts9sim       # any effect: its settings, ranges and options
```

These commands start a hidden Guitarix and never make sound. They're safe while the user is playing.

## 4. Write the preset

Match the style of the existing entries in `presets.json` (2-space indent, keys in this order):

```json
{
  "artist": "Pink Floyd",
  "song": "Comfortably Numb",
  "album": "The Wall (1979)",
  "parts": [
    {
      "name": "Second solo (fuzz lead)",
      "description": "Gilmour-style: Big Muff into a clean, loud amp with long delay. Bend wide and let notes sing.",
      "level_db": 0,
      "guitar": { "pickup": "bridge", "volume": 10, "tone1": 10, "tone2": 8 },
      "amp": { "model": "pre 12AT7/ push pull 6V6", "pregain": -2, "drive": -4 },
      "fx": {
        "muff": { "Tone": 0.55, "Level": 0.5 },
        "amp.tonestack": { "select": "Twin Reverb", "Bass": 0.45, "Middle": 0.55, "Treble": 0.6 },
        "cab": { "select": "4x12" },
        "echo": { "bpm": 94, "percent": 30 },
        "zita_rev1": { "output.dry_wet_mix": -0.45 }
      }
    }
  ]
}
```

- **`name`**: short, says which part and what kind of sound: "Main riff (heavy)", "Solo (wah lead)".
- **`description`**: one or two sentences saying what the chain does and any playing tip (pick attack,
  volume swells, tuning). Say "X-style" or "inspired by": these are approximations, never claim it is the
  exact original rig.
- Leave `level_db` at 0 for now. Step 6 sets it.
- **`guitar`**: how to set the user's Strat (5-way selector, Volume, Tone 1 = neck, Tone 2 = middle/bridge),
  shown under the chain. `pickup` is one of `bridge`, `bridge+middle`, `middle`, `middle+neck`, `neck`; knobs
  are 0..10. Translate the original guitarist's pickup choice (Les Paul neck humbucker → `neck`, bridge → `bridge`;
  Strat "quack" → `bridge+middle`/`middle+neck`), and roll a tone knob back for darker "woman tone" leads.
- Don't set `on_off`, `pp`, `position` or `s_h`: lazyguitarist manages those.

## 5. Validate

```bash
./lazyguitarist --validate
```

Fix every ✗ line (wrong names, out-of-range values, options that don't exist) and re-run until all parts
show ✓.

## 6. Calibrate loudness

```bash
./lazyguitarist --calibrate "<song name>"
```

It plays a test riff through a hidden Guitarix (silent, ~12 s per part) and prints a `"level_db"` for
each part, so the new song is as loud as the others. Copy those values into the new parts. Run
`--validate` once more.

## 7. Update the README

Add the new parts to the **Included presets** table in `README.md`. The `Key` column restarts at 1
for each song (number keys pick parts of the current song), with a short chain summary like the existing rows.

## 8. Tell the user

Give a short summary:

- the song and parts added, with each chain in one line;
- any non-standard tuning;
- that they need to restart `lazyguitarist` (or `go build -o lazyguitarist .` first if they run the binary), then
  pick the song and press `1`, `2`, `3`… for its parts.

The presets are educated guesses from research. Invite them to say "darker", "more gain", "less
echo", etc., and adjust the preset, then re-run steps 5–6.
