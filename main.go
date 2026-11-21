// lazyguitarist: a terminal guitar multi-effect for Linux.
//
// Guitar -> USB audio dongle -> Guitarix (headless DSP engine) -> wired headphones.
// lazyguitarist starts Guitarix without its GUI, wires it with PipeWire, loads song presets
// over Guitarix's JSON-RPC port, and has a tuner and a clean recorder.
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

//go:embed presets.json
var embeddedPresets []byte

// loadPresets: --presets flag, then ~/.config/lazyguitarist/presets.json, then presets.json
// next to the binary, then the copy built into the binary.
func loadPresets(flagPath string) ([]Song, string, error) {
	var candidates []string
	if flagPath != "" {
		candidates = []string{flagPath}
	} else {
		candidates = append(candidates, filepath.Join(configDir, "presets.json"))
		if exe, err := os.Executable(); err == nil {
			if real, err := filepath.EvalSymlinks(exe); err == nil {
				candidates = append(candidates, filepath.Join(filepath.Dir(real), "presets.json"))
			}
		}
	}
	for _, c := range candidates {
		b, err := os.ReadFile(c)
		if err != nil {
			if flagPath != "" {
				return nil, c, err
			}
			continue
		}
		var pf PresetFile
		if err := json.Unmarshal(b, &pf); err != nil {
			return nil, c, fmt.Errorf("%s: %w", c, err)
		}
		return pf.Songs, c, nil
	}
	var pf PresetFile
	err := json.Unmarshal(embeddedPresets, &pf)
	return pf.Songs, "built-in", err
}

func check() {
	fmt.Println("Inputs (sources):")
	srcs := Sources()
	for _, s := range srcs {
		fmt.Printf("  %s\n      %s  %d Hz  vol %d%%\n", s.Name, s.Desc, s.Rate, s.Volume)
	}
	fmt.Println("Outputs (sinks):")
	sinks := Sinks()
	for _, s := range sinks {
		fmt.Printf("  %s\n      %s\n", s.Name, s.Desc)
	}
	cfg := loadConfig()
	in := pickInput(srcs, cfg.Input)
	fmt.Printf("\nlazyguitarist would use:\n  input  %s\n  output %s\n", in, pickOutput(sinks, in, cfg.Output))
	_, from, err := loadPresets("")
	fmt.Printf("  presets %s %v\n", from, errOr(err))
	for _, t := range []string{"guitarix", "pw-jack", "pw-link", "pw-metadata", "pw-record", "pactl"} {
		st := "ok"
		if _, err := exec.LookPath(t); err != nil {
			st = "MISSING"
		}
		fmt.Printf("  %-12s %s\n", t, st)
	}
}

func errOr(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

func main() {
	migrateOldName()
	doCheck := flag.Bool("check", false, "list audio devices and exit")
	startTuner := flag.Bool("tuner", false, "open straight into the tuner")
	presetsPath := flag.String("presets", "", "presets file (default: ~/.config/lazyguitarist/presets.json, then next to the binary)")
	paramsOf := flag.String("params", "", "show the settings of an effect (or \"units\" for the list) and exit")
	doValidate := flag.Bool("validate", false, "check presets.json against Guitarix and exit")
	doCalibrate := flag.Bool("calibrate", false, "suggest level_db for songs matching the argument (or all) and exit")
	flag.Parse()

	if *doCheck {
		check()
		return
	}
	if *paramsOf != "" {
		os.Exit(paramsCmd(*paramsOf))
	}
	songs, from, err := loadPresets(*presetsPath)
	if err != nil || len(songs) == 0 {
		fmt.Fprintf(os.Stderr, "lazyguitarist: could not load presets (%s): %v\n", from, err)
		os.Exit(1)
	}
	if *doValidate {
		os.Exit(validateCmd(songs, from))
	}
	if *doCalibrate {
		os.Exit(calibrateCmd(songs, strings.Join(flag.Args(), " ")))
	}

	cfg := loadConfig()
	rig := NewRig(cfg)
	fmt.Println("starting Guitarix…")
	if err := rig.Start(); err != nil {
		rig.Stop()
		fmt.Fprintf(os.Stderr, "lazyguitarist: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(newModel(rig, songs, *startTuner), tea.WithAltScreen())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGHUP)
	go func() { <-sig; p.Quit() }()

	_, runErr := p.Run()
	path, d := rig.Stop()
	cfg.Save()
	if path != "" {
		fmt.Printf("saved recording (%s): %s\n", fmtDur(d), path)
	}
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "lazyguitarist: %v\n", runErr)
		os.Exit(1)
	}
}
