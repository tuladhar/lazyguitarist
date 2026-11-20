---
name: preset
description: Add a guitar tone preset for a song to guitarfx's presets.json. Use when the user runs /preset <song name> or asks to add, create or make a preset or tone for a song, solo or riff (e.g. "/preset Comfortably Numb", "add a preset for Iron Man", "make a tone for the Hotel California solo").
argument-hint: <song name> [artist] [part, e.g. "solo"]
---

# Add a song preset to guitarfx

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
  `description`, since guitarfx's tuner shows standard tuning.

## 3. Map the rig onto guitarfx's building blocks

Only these effects exist (in signal order). Run `./guitarfx --params units` to confirm:

| Block | Use for |
|---|---|
| `noise_gate` | any high-gain part (threshold ~0.02–0.04) |
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
go build -o guitarfx .
./guitarfx --params amp          # amp models, gain ranges
./guitarfx --params cab          # cab options
./guitarfx --params ts9sim       # any effect: its settings, ranges and options
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
- Don't set `on_off`, `pp`, `position` or `s_h`: guitarfx manages those.

## 5. Validate

```bash
./guitarfx --validate
```

Fix every ✗ line (wrong names, out-of-range values, options that don't exist) and re-run until all parts
show ✓.

## 6. Calibrate loudness

```bash
./guitarfx --calibrate "<song name>"
```

It plays a test riff through a hidden Guitarix (silent, ~12 s per part) and prints a `"level_db"` for
each part, so the new song is as loud as the others. Copy those values into the new parts. Run
`--validate` once more.

## 7. Update the README

Add the new parts to the **Included presets** table in `README.md`. Number them continuing from the last
row, with a short chain summary like the existing rows.

## 8. Tell the user

Give a short summary:

- the song and parts added, with each chain in one line;
- any non-standard tuning;
- that they need to restart `guitarfx` (or `go build -o guitarfx .` first if they run the binary), then
  press the new numbers to play.

The presets are educated guesses from research. Invite them to say "darker", "more gain", "less
echo", etc., and adjust the preset, then re-run steps 5–6.
