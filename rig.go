package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------- presets & config

type Part struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	LevelDB     float64                   `json:"level_db"`
	Guitar      *Guitar                   `json:"guitar,omitempty"`
	Amp         map[string]any            `json:"amp"`
	FX          map[string]map[string]any `json:"fx"`
}

// Guitar is how to set the guitar itself for a part: a Strat-style 5-way pickup selector, one volume knob and two
// tone knobs (tone1 = neck, tone2 = middle/bridge), each 0..10.
type Guitar struct {
	Pickup string  `json:"pickup"`
	Volume float64 `json:"volume"`
	Tone1  float64 `json:"tone1"`
	Tone2  float64 `json:"tone2"`
}

// pickupPositions are the 5-way selector positions, bridge first (position 1).
var pickupPositions = []string{"bridge", "bridge+middle", "middle", "middle+neck", "neck"}

func pickupIndex(p string) int {
	for i, v := range pickupPositions {
		if v == p {
			return i
		}
	}
	return -1
}

type Song struct {
	Artist string `json:"artist"`
	Song   string `json:"song"`
	Album  string `json:"album"`
	Parts  []Part `json:"parts"`
}

type PresetFile struct {
	Songs []Song `json:"songs"`
}

type Config struct {
	Input    string   `json:"input,omitempty"`
	Output   string   `json:"output,omitempty"`
	VolumeDB *float64 `json:"volume_db,omitempty"`
	Quantum  int      `json:"quantum,omitempty"`
	Last     []string `json:"last,omitempty"`
}

var (
	home       = os.Getenv("HOME")
	configDir  = filepath.Join(home, ".config", "lazyguitarix")
	configFile = filepath.Join(configDir, "config.json")
	logFile    = filepath.Join(home, ".cache", "lazyguitarix", "guitarix.log")
	recDir     = filepath.Join(home, "Music", "lazyguitarix")
	quantums   = []int{64, 128, 256}
)

func loadConfig() *Config {
	c := &Config{}
	if b, err := os.ReadFile(configFile); err == nil {
		_ = json.Unmarshal(b, c)
	}
	return c
}

func (c *Config) Save() {
	_ = os.MkdirAll(configDir, 0o755)
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := configFile + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, configFile)
	}
}

// ---------------------------------------------------------------- effect chain

// unit is an effect lazyguitarix manages. pp "" means Guitarix keeps its own fixed slot
// (noise gate first, reverb in the stereo rack); otherwise pre/post amp at pos.
type unit struct {
	id, label, pp string
	pos           int
}

// In signal order; the amp sits between the last "pre" and first "post" unit.
var units = []unit{
	{"abgate", "gate", "pre", 0},
	{"crybaby", "wah", "pre", 1},
	{"compressor", "comp", "pre", 2},
	{"rangem", "treble boost", "pre", 3},
	{"ts9sim", "TS9", "pre", 4},
	{"muff", "Big Muff", "pre", 5},
	{"amp.tonestack", "tonestack", "post", 1},
	{"cab", "cab", "post", 2},
	{"echo", "echo", "post", 3},
	{"zita_rev1", "reverb", "", 0},
}

var ampKeys = map[string]string{"model": "tube.select", "pregain": "amp2.stage1.Pregain", "drive": "amp2.stage2.gain1"}

// A clean gain stage at the very end of the mono chain carries each part's level_db.
const levelUnit, levelPos = "gain", 9

func isUnit(id string) bool {
	for _, u := range units {
		if u.id == id {
			return true
		}
	}
	return false
}

// chainLabels returns the effect chain of a part in signal order.
func chainLabels(p *Part) []string {
	var out []string
	ampDone := false
	for _, u := range units {
		if !ampDone && (u.pp == "post" || u.id == "zita_rev1") {
			out = append(out, "amp "+fmt.Sprint(p.Amp["model"]))
			ampDone = true
		}
		params, ok := p.FX[u.id]
		if !ok {
			continue
		}
		label := u.label
		if bpm, ok := params["bpm"].(float64); ok && u.id == "echo" && bpm > 0 {
			label += fmt.Sprintf(" %dms", int(math.Round(60000/bpm)))
		}
		out = append(out, label)
	}
	if !ampDone {
		out = append(out, "amp "+fmt.Sprint(p.Amp["model"]))
	}
	return out
}

// ---------------------------------------------------------------- the rig

type Rig struct {
	cfg        *Config
	gx         Guitarix
	oldQuantum int
	Input      string
	Output     string
	links      [][2]string
	Sources    []Device
	Sinks      []Device
	CurSong    *Song
	CurPart    *Part
	levelDB    float64
	Bypass     bool
	Status     string
	StatusBad  bool
	Rec        Recorder
	gain       int
	gainAt     time.Time
}

func NewRig(cfg *Config) *Rig { return &Rig{cfg: cfg, oldQuantum: -1, gain: -1} }

func (r *Rig) Quantum() int {
	for _, q := range quantums {
		if q == r.cfg.Quantum {
			return q
		}
	}
	return 128
}

func (r *Rig) LatencyMs() float64 { return float64(r.Quantum()) / rate * 1000 }

func (r *Rig) Start() error {
	_ = os.MkdirAll(filepath.Dir(logFile), 0o755)
	r.oldQuantum = GetForceQuantum()
	SetForceQuantum(r.Quantum())
	if err := r.gx.Start(r.Quantum(), logFile); err != nil {
		return err
	}
	Link(gxName+"_amp:out_0", gxName+"_fx:in_0")
	r.gx.Set(map[string]any{"engine.mute": 0, "engine.bypass": 0, "tuner.on_off": 1})
	r.Rewire(r.cfg.Input, r.cfg.Output)
	return nil
}

// Stop ends any recording and shuts everything down; returns a saved recording path, if any.
func (r *Rig) Stop() (string, time.Duration) {
	path, d := r.Rec.Stop()
	r.gx.Stop()
	if r.oldQuantum >= 0 {
		SetForceQuantum(r.oldQuantum)
		r.oldQuantum = -1
	}
	return path, d
}

func (r *Rig) setStatus(bad bool, format string, a ...any) {
	r.Status, r.StatusBad = fmt.Sprintf(format, a...), bad
}

func (r *Rig) Rewire(wantIn, wantOut string) {
	for _, l := range r.links {
		Unlink(l[0], l[1])
	}
	r.links = nil
	r.Sources, r.Sinks = Sources(), Sinks()
	r.Input = pickInput(r.Sources, wantIn)
	r.Output = pickOutput(r.Sinks, r.Input, wantOut)
	var want [][2]string
	cp := ""
	if r.Input != "" {
		cp = capturePort(r.Input)
	}
	if cp != "" {
		want = append(want, [2]string{cp, gxName + "_amp:in_0"})
	}
	left, right := "", ""
	if r.Output != "" {
		left, right = playbackPorts(r.Output)
	}
	if left != "" {
		want = append(want, [2]string{gxName + "_fx:out_0", left}, [2]string{gxName + "_fx:out_1", right})
	}
	for _, l := range want {
		if Link(l[0], l[1]) {
			r.links = append(r.links, l)
		}
	}
	r.cfg.Input, r.cfg.Output = r.Input, r.Output
	r.gain, r.gainAt = -1, time.Time{}
	switch {
	case cp == "":
		r.setStatus(true, "No guitar input found. Plug in the USB dongle and press i.")
	case left == "":
		r.setStatus(true, "No output found. Press o.")
	}
}

func (r *Rig) Cycle(input bool) {
	devs, cur := Sinks(), r.Output
	if input {
		devs, cur = Sources(), r.Input
	}
	if len(devs) == 0 {
		return
	}
	next := devs[0].Name
	for i, d := range devs {
		if d.Name == cur {
			next = devs[(i+1)%len(devs)].Name
		}
	}
	if input {
		r.Rewire(next, r.Output)
	} else {
		r.Rewire(r.Input, next)
	}
}

// InputGain returns the dongle's input volume in percent (cached, -1 if unknown).
func (r *Rig) InputGain() int {
	if time.Since(r.gainAt) > 2*time.Second {
		r.gain = -1
		for _, s := range Sources() {
			if s.Name == r.Input {
				r.gain = s.Volume
			}
		}
		r.gainAt = time.Now()
	}
	return r.gain
}

func (r *Rig) ChangeInputGain(step int) {
	g := r.InputGain()
	if g < 0 {
		return
	}
	g = max(0, min(150, g+step))
	SetSourceVolume(r.Input, g)
	r.gain, r.gainAt = g, time.Now()
}

func (r *Rig) VolumeDB() float64 {
	if r.cfg.VolumeDB == nil {
		return -12
	}
	return *r.cfg.VolumeDB
}

func (r *Rig) ChangeVolume(step float64) {
	v := math.Max(-40, math.Min(0, r.VolumeDB()+step))
	r.cfg.VolumeDB = &v
	r.applyVolume()
}

func (r *Rig) applyVolume() {
	r.gx.Set(map[string]any{"amp.out_master": math.Max(-50, math.Min(4, r.VolumeDB()))})
}

func (r *Rig) SetQuantum(q int) {
	r.cfg.Quantum = q
	SetForceQuantum(q)
}

func (r *Rig) Apply(s *Song, p *Part) {
	off := map[string]any{}
	for _, u := range units {
		off[u.id+".on_off"] = 0
	}
	r.gx.Set(off) // everything off first

	vals := map[string]any{}
	for _, u := range units {
		if u.pp != "" {
			vals[u.id+".pp"], vals[u.id+".position"] = u.pp, u.pos
		}
	}
	for k, id := range ampKeys {
		if v, ok := p.Amp[k]; ok {
			vals[id] = v
		}
	}
	var unknown []string
	for id, params := range p.FX {
		if !isUnit(id) {
			unknown = append(unknown, "effect "+id)
			continue
		}
		for k, v := range params {
			vals[id+"."+k] = v
		}
	}
	for _, b := range r.gx.Set(vals) {
		unknown = append(unknown, "param "+b)
	}

	r.levelDB = math.Max(-40, math.Min(40, p.LevelDB))
	r.gx.Set(map[string]any{levelUnit + ".pp": "post", levelUnit + ".position": levelPos, levelUnit + ".gain": r.levelDB})
	on := map[string]any{levelUnit + ".on_off": 1}
	for id := range p.FX {
		if isUnit(id) {
			on[id+".on_off"] = 1
		}
	}
	r.gx.Set(on)
	r.Bypass = false
	r.gx.Set(map[string]any{"engine.bypass": 0, "ampstack.on_off": 1})
	r.applyVolume()
	r.CurSong, r.CurPart = s, p
	r.cfg.Last = []string{s.Song, p.Name}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		r.setStatus(true, "presets.json: unknown %s", strings.Join(unknown, ", "))
	} else {
		r.Status = ""
	}
}

func (r *Rig) ToggleBypass() {
	r.Bypass = !r.Bypass
	b := 0
	if r.Bypass {
		b = 1
	}
	r.gx.Set(map[string]any{"engine.bypass": b})
}

func (r *Rig) Mute(m bool) {
	v := 0
	if m {
		v = 1
	}
	r.gx.Set(map[string]any{"engine.mute": v})
}

func (r *Rig) ToggleRecording() {
	if r.Rec.Active() {
		path, d := r.Rec.Stop()
		r.setStatus(false, "Saved %s recording → %s", fmtDur(d), path)
		return
	}
	label := "take"
	if r.CurPart != nil {
		label = r.CurSong.Song + " " + strings.SplitN(r.CurPart.Name, "(", 2)[0]
	}
	if err := r.Rec.Start(recDir, label, gxName+"_fx:out_0", gxName+"_fx:out_1"); err != nil {
		r.setStatus(true, "Recording failed: %v", err)
		return
	}
	r.Status = ""
}

// ---------------------------------------------------------------- tuner math

var (
	noteNames      = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	standardTuning = []int{40, 45, 50, 55, 59, 64} // E2 A2 D3 G3 B3 E4
	stringNames    = []string{"6th string · low E", "5th string · A", "4th string · D",
		"3rd string · G", "2nd string · B", "1st string · high E"}
)

// Guitar range; the floor also rejects 50/60 Hz mains hum on the cable.
const tunerMinHz, tunerMaxHz = 60.0, 1400.0

func noteName(midi int) string { return fmt.Sprintf("%s%d", noteNames[((midi%12)+12)%12], midi/12-1) }

type TunerReading struct {
	Freq     float64
	Played   string // nearest chromatic note
	StringIx int    // nearest standard-tuning string
	Cents    int    // vs that string; negative = flat
}

func readTuner(f float64) TunerReading {
	exact := 69 + 12*math.Log2(f/440)
	best := 0
	for i, m := range standardTuning {
		if math.Abs(float64(m)-exact) < math.Abs(float64(standardTuning[best])-exact) {
			best = i
		}
	}
	return TunerReading{
		Freq:     f,
		Played:   noteName(int(math.Round(exact))),
		StringIx: best,
		Cents:    int(math.Round((exact - float64(standardTuning[best])) * 100)),
	}
}
