package main

// Preset tooling: --params (what can I set?), --validate (is presets.json right?) and
// --calibrate (what level_db makes this part as loud as the others?).
//
// All of them run a separate, hidden Guitarix instance that is never connected to your
// speakers or headphones, so they're safe to run while lazyguitarix is playing.

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	calTargetRMS = -18.0 // dBFS RMS every part is matched to
	calMaxPeak   = -1.0  // ...unless its peaks would go above this (you play at -12 dB volume, so there is headroom)
)

// startHidden runs Guitarix under its own name with nothing wired to the outside world.
func startHidden(name string) (*Rig, error) {
	gxName, recNode = name, name+"_rec"
	zero := 0.0
	r := NewRig(&Config{VolumeDB: &zero})
	_ = os.MkdirAll(filepath.Dir(logFile), 0o755)
	if err := r.gx.Start(256, filepath.Join(filepath.Dir(logFile), name+".log")); err != nil {
		r.gx.Stop()
		return nil, err
	}
	Link(gxName+"_amp:out_0", gxName+"_fx:in_0")
	r.gx.Set(map[string]any{"engine.mute": 0, "engine.bypass": 0})
	return r, nil
}

func stopHidden(r *Rig) {
	r.gx.Stop()
	_ = os.Remove(filepath.Join(home, ".config", "guitarix", gxName+"_rc"))
}

// ---------------------------------------------------------------- --params

func paramsCmd(unitID string) int {
	if unitID == "units" || unitID == "" {
		fmt.Println("Effects a preset can use (in signal order):")
		fmt.Printf("  %-14s %s\n", "amp", "amp model + gain (the \"amp\" block of a part)")
		for _, u := range units {
			where := "fixed slot"
			if u.pp != "" {
				where = u.pp + " amp"
			}
			fmt.Printf("  %-14s %-13s (%s)\n", u.id, u.label, where)
		}
		fmt.Println("\nRun lazyguitarix --params <effect> to see its settings.")
		return 0
	}
	r, err := startHidden("lazyguitarix_params")
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazyguitarix:", err)
		return 1
	}
	defer stopHidden(r)

	if unitID == "amp" {
		keys := []string{"model", "pregain", "drive"}
		for _, k := range keys {
			printParam(k, r.gx.params[ampKeys[k]])
		}
		return 0
	}
	if !isUnit(unitID) {
		fmt.Fprintf(os.Stderr, "unknown effect %q (try: lazyguitarix --params units)\n", unitID)
		return 1
	}
	var ids []string
	for id := range r.gx.params {
		if strings.HasPrefix(id, unitID+".") {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		key := strings.TrimPrefix(id, unitID+".")
		if managedKey(key) {
			continue
		}
		printParam(key, r.gx.params[id])
	}
	return 0
}

// managedKey: settings lazyguitarix controls itself; presets must not set them.
func managedKey(k string) bool {
	return k == "on_off" || k == "pp" || k == "position" || k == "s_h"
}

func printParam(key string, p ParamInfo) {
	switch {
	case len(p.Options) > 0:
		fmt.Printf("  %-26s one of: %s\n", key, strings.Join(quoteAll(p.Options), ", "))
	case p.Lower != nil && p.Upper != nil:
		fmt.Printf("  %-26s %s %g .. %g   (default %v)\n", key, strings.ToLower(p.Type), *p.Lower, *p.Upper, p.Default)
	default:
		fmt.Printf("  %-26s %s   (default %v)\n", key, strings.ToLower(p.Type), p.Default)
	}
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

// ---------------------------------------------------------------- --validate

func checkValue(p ParamInfo, v any) string {
	if len(p.Options) > 0 {
		if s, ok := v.(string); ok {
			for _, o := range p.Options {
				if o == s {
					return ""
				}
			}
			return fmt.Sprintf("%q is not an option (one of: %s)", s, strings.Join(quoteAll(p.Options), ", "))
		}
		if p.Type != "FloatEnum" {
			return fmt.Sprintf("must be one of: %s", strings.Join(quoteAll(p.Options), ", "))
		}
	}
	switch p.Type {
	case "Bool":
		if f, ok := v.(float64); ok && (f == 0 || f == 1) {
			return ""
		}
		if _, ok := v.(bool); ok {
			return ""
		}
		return "must be 0 or 1"
	case "String":
		if _, ok := v.(string); ok {
			return ""
		}
		return "must be a string"
	}
	f, ok := v.(float64)
	if !ok {
		return fmt.Sprintf("must be a number, got %v", v)
	}
	if p.Lower != nil && p.Upper != nil && (f < *p.Lower || f > *p.Upper) {
		return fmt.Sprintf("%g is out of range %g .. %g", f, *p.Lower, *p.Upper)
	}
	return ""
}

func validateCmd(songs []Song, from string) int {
	r, err := startHidden("lazyguitarix_check")
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazyguitarix:", err)
		return 1
	}
	defer stopHidden(r)
	params := r.gx.params

	fmt.Printf("Checking %s\n\n", from)
	bad := 0
	for _, s := range songs {
		if s.Song == "" || s.Artist == "" || len(s.Parts) == 0 {
			fmt.Printf("✗ %q: needs artist, song and at least one part\n", s.Song)
			bad++
			continue
		}
		for _, p := range s.Parts {
			var probs []string
			if p.Name == "" {
				probs = append(probs, "part has no name")
			}
			if p.LevelDB < -40 || p.LevelDB > 40 {
				probs = append(probs, fmt.Sprintf("level_db %g is out of range -40 .. 40", p.LevelDB))
			}
			if g := p.Guitar; g != nil {
				if pickupIndex(g.Pickup) < 0 {
					probs = append(probs, fmt.Sprintf("guitar.pickup %q: use one of %s", g.Pickup, strings.Join(pickupPositions, ", ")))
				}
				for k, v := range map[string]float64{"volume": g.Volume, "tone1": g.Tone1, "tone2": g.Tone2} {
					if v < 0 || v > 10 {
						probs = append(probs, fmt.Sprintf("guitar.%s %g is out of range 0 .. 10", k, v))
					}
				}
			}
			for k, v := range p.Amp {
				id, ok := ampKeys[k]
				if !ok {
					probs = append(probs, fmt.Sprintf("amp.%s: unknown (use model, pregain, drive)", k))
					continue
				}
				if msg := checkValue(params[id], v); msg != "" {
					probs = append(probs, fmt.Sprintf("amp.%s: %s", k, msg))
				}
			}
			if _, ok := p.Amp["model"]; !ok {
				probs = append(probs, "amp.model is missing")
			}
			for u, ps := range p.FX {
				if !isUnit(u) {
					probs = append(probs, fmt.Sprintf("fx.%s: unknown effect (lazyguitarix --params units)", u))
					continue
				}
				for k, v := range ps {
					if managedKey(k) {
						probs = append(probs, fmt.Sprintf("fx.%s.%s: set by lazyguitarix, remove it", u, k))
						continue
					}
					info, ok := params[u+"."+k]
					if !ok {
						probs = append(probs, fmt.Sprintf("fx.%s.%s: unknown setting (lazyguitarix --params %s)", u, k, u))
						continue
					}
					if msg := checkValue(info, v); msg != "" {
						probs = append(probs, fmt.Sprintf("fx.%s.%s: %s", u, k, msg))
					}
				}
			}
			label := fmt.Sprintf("%s - %s › %s", s.Artist, s.Song, p.Name)
			if len(probs) == 0 {
				fmt.Println("✓", label)
				continue
			}
			sort.Strings(probs)
			fmt.Println("✗", label)
			for _, pr := range probs {
				fmt.Println("    ", pr)
			}
			bad++
		}
	}
	if bad > 0 {
		fmt.Printf("\n%d part(s) need fixing.\n", bad)
		return 1
	}
	fmt.Println("\nAll presets are valid.")
	return 0
}

// ---------------------------------------------------------------- --calibrate

func calibrateCmd(songs []Song, filter string) int {
	var targets []*Song
	for i := range songs {
		s := &songs[i]
		hay := strings.ToLower(s.Artist + " " + s.Song)
		if filter == "" || filter == "all" || strings.Contains(hay, strings.ToLower(filter)) {
			targets = append(targets, s)
		}
	}
	if len(targets) == 0 {
		fmt.Fprintf(os.Stderr, "no song matches %q\n", filter)
		return 1
	}
	r, err := startHidden("lazyguitarix_cal")
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazyguitarix:", err)
		return 1
	}
	defer stopHidden(r)
	tmp, err := os.MkdirTemp("", "lazyguitarix-cal-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "lazyguitarix:", err)
		return 1
	}
	defer os.RemoveAll(tmp)
	pluck := filepath.Join(tmp, "pluck.wav")
	if err := writePluck(pluck); err != nil {
		fmt.Fprintln(os.Stderr, "lazyguitarix:", err)
		return 1
	}

	fmt.Printf("Matching every part to %.0f dB RMS (peaks kept under %.0f dB). Plays a test riff through\n"+
		"a hidden Guitarix; nothing goes to your speakers. ~12 s per part.\n\n", calTargetRMS, calMaxPeak)
	for _, s := range targets {
		fmt.Printf("%s - %s\n", s.Artist, s.Song)
		for pi := range s.Parts {
			part := s.Parts[pi] // copy; we only print suggestions
			level := part.LevelDB
			var rms, peak float64
			for round := 0; round < 4; round++ {
				part.LevelDB = level
				r.Apply(s, &part)
				time.Sleep(700 * time.Millisecond) // let the previous part's tails die
				rms, peak, err = measure(filepath.Join(tmp, "out.wav"), pluck)
				if err != nil {
					fmt.Fprintln(os.Stderr, "lazyguitarix:", err)
					return 1
				}
				next := math.Min(level+(calTargetRMS-rms), level+(calMaxPeak-peak))
				next = math.Max(-40, math.Min(40, math.Round(next*10)/10))
				if math.Abs(next-level) < 0.3 {
					break
				}
				level = next
			}
			fmt.Printf("  \"level_db\": %-6.1f  %-34s (rms %.1f dB, peak %.1f dB; was %.1f)\n",
				level, part.Name, rms, peak, s.Parts[pi].LevelDB)
		}
	}
	return 0
}

// measure plays the test riff through the hidden Guitarix, records it, and returns RMS and peak in dBFS.
func measure(outPath, pluck string) (rms, peak float64, err error) {
	recName, playName := gxName+"_calrec", gxName+"_calplay"
	rec := exec.Command("pw-record", "-P", "{ node.autoconnect=false node.name="+recName+" }",
		"--rate", "48000", "--channels", "2", "--format", "s32", outPath)
	if err = rec.Start(); err != nil {
		return
	}
	defer func() { _ = rec.Process.Kill(); _ = rec.Wait() }()
	if err = waitPort(false, recName+":input_FR"); err != nil {
		return
	}
	Link(gxName+"_fx:out_0", recName+":input_FL")
	Link(gxName+"_fx:out_1", recName+":input_FR")

	play := exec.Command("pw-play", "-P", "{ node.autoconnect=false node.name="+playName+" }", pluck)
	if err = play.Start(); err != nil {
		return
	}
	if err = waitPort(true, playName+":output_"); err != nil {
		_ = play.Process.Kill()
		return
	}
	for _, p := range Ports(true) {
		if strings.HasPrefix(p, playName+":output_") {
			Link(p, gxName+"_amp:in_0")
			break
		}
	}
	_ = play.Wait()
	time.Sleep(1200 * time.Millisecond) // echo/reverb tails
	_ = rec.Process.Signal(os.Interrupt)
	_ = rec.Wait()

	samples, err := readWavLeft32(outPath)
	if err != nil {
		return
	}
	var sum, pk float64
	for _, v := range samples {
		f := float64(v) / 2147483648.0
		sum += f * f
		pk = math.Max(pk, math.Abs(f))
	}
	if len(samples) == 0 {
		return 0, 0, fmt.Errorf("empty recording")
	}
	return 20 * math.Log10(math.Sqrt(sum/float64(len(samples)))+1e-9), 20 * math.Log10(pk+1e-9), nil
}

func waitPort(outputs bool, prefix string) error {
	deadline := time.Now().Add(3 * time.Second)
	for !hasPort(outputs, prefix) {
		if time.Now().After(deadline) {
			return fmt.Errorf("port %s did not appear", prefix)
		}
		time.Sleep(30 * time.Millisecond)
	}
	return nil
}

// writePluck writes a 4 s mono test riff (plucked-string synthesis, fixed seed) peaking at -10.5 dBFS,
// roughly what a guitar gives on a USB mic input.
func writePluck(path string) error {
	const sr = 48000
	out := make([]float64, sr*4)
	seed := uint32(1)
	rnd := func() float64 { seed = seed*1664525 + 1013904223; return float64(seed)/2147483648.0 - 1 }
	notes := []struct{ t, f float64 }{{0.5, 82.4}, {0.5, 123.5}, {0.5, 164.8}, {1.6, 110}, {1.6, 164.8}, {2.6, 196}, {2.6, 293.7}}
	for _, n := range notes {
		N := int(sr / n.f)
		buf := make([]float64, N)
		for i := range buf {
			buf[i] = rnd()
		}
		start := int(n.t * sr)
		for i := 0; i < sr*3/2 && start+i < len(out); i++ {
			v := buf[i%N]
			buf[i%N] = 0.497 * (v + buf[(i+1)%N])
			out[start+i] += v * 0.33
		}
	}
	var m float64
	for _, v := range out {
		m = math.Max(m, math.Abs(v))
	}
	data := make([]byte, len(out)*2)
	for i, v := range out {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(int16(v/m*0.3*32767)))
	}
	hdr := make([]byte, 44)
	copy(hdr[0:], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], uint32(36+len(data)))
	copy(hdr[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(hdr[16:], 16)
	binary.LittleEndian.PutUint16(hdr[20:], 1)
	binary.LittleEndian.PutUint16(hdr[22:], 1)
	binary.LittleEndian.PutUint32(hdr[24:], sr)
	binary.LittleEndian.PutUint32(hdr[28:], sr*2)
	binary.LittleEndian.PutUint16(hdr[32:], 2)
	binary.LittleEndian.PutUint16(hdr[34:], 16)
	copy(hdr[36:], "data")
	binary.LittleEndian.PutUint32(hdr[40:], uint32(len(data)))
	return os.WriteFile(path, append(hdr, data...), 0o644)
}

// readWavLeft32 returns the first channel of a 32-bit PCM WAV.
func readWavLeft32(path string) ([]int32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil || len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, fmt.Errorf("bad recording")
	}
	channels, bits := 0, 0
	for p := 12; p+8 <= len(b); {
		id, size := string(b[p:p+4]), int(binary.LittleEndian.Uint32(b[p+4:p+8]))
		body := p + 8
		if id == "fmt " && body+16 <= len(b) {
			channels = int(binary.LittleEndian.Uint16(b[body+2:]))
			bits = int(binary.LittleEndian.Uint16(b[body+14:]))
		}
		if id == "data" {
			if bits != 32 || channels == 0 {
				return nil, fmt.Errorf("unexpected recording format")
			}
			end := body + size
			if size == 0 || size == -1 || end > len(b) || end < body {
				end = len(b)
			}
			var out []int32
			for q := body; q+4*channels <= end; q += 4 * channels {
				out = append(out, int32(binary.LittleEndian.Uint32(b[q:])))
			}
			return out, nil
		}
		p = body + size + size%2
	}
	return nil, fmt.Errorf("no audio in recording")
}
