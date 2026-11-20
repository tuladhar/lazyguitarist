package main

// PipeWire helpers: device lists (pactl), wiring (pw-link), buffer size (pw-metadata).

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type Device struct {
	Name   string
	Desc   string
	Rate   int
	Volume int // percent, -1 if unknown
}

func output(name string, args ...string) string {
	out, _ := exec.Command(name, args...).Output()
	return string(out)
}

type pactlDev struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	SampleSpec  string                     `json:"sample_specification"`
	Volume      map[string]json.RawMessage `json:"volume"`
}

var rateRe = regexp.MustCompile(`(\d+)Hz`)

func pactlList(kind string) []pactlDev {
	var devs []pactlDev
	_ = json.Unmarshal([]byte(output("pactl", "-f", "json", "list", kind)), &devs)
	return devs
}

func toDevice(d pactlDev) Device {
	dev := Device{Name: d.Name, Desc: d.Description, Volume: -1}
	if dev.Desc == "" {
		dev.Desc = d.Name
	}
	if m := rateRe.FindStringSubmatch(d.SampleSpec); m != nil {
		dev.Rate, _ = strconv.Atoi(m[1])
	}
	for _, raw := range d.Volume {
		var ch struct {
			ValuePercent string `json:"value_percent"`
		}
		if json.Unmarshal(raw, &ch) == nil && ch.ValuePercent != "" {
			dev.Volume, _ = strconv.Atoi(strings.TrimSuffix(ch.ValuePercent, "%"))
			break
		}
	}
	return dev
}

func Sources() []Device {
	var out []Device
	for _, d := range pactlList("sources") {
		if !strings.HasSuffix(d.Name, ".monitor") {
			out = append(out, toDevice(d))
		}
	}
	return out
}

func Sinks() []Device {
	var out []Device
	for _, d := range pactlList("sinks") {
		out = append(out, toDevice(d))
	}
	return out
}

func SetSourceVolume(name string, pct int) {
	_ = exec.Command("pactl", "set-source-volume", name, strconv.Itoa(pct)+"%").Run()
}

func Ports(outputs bool) []string {
	flag := "-i"
	if outputs {
		flag = "-o"
	}
	var ports []string
	for _, l := range strings.Split(output("pw-link", flag), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			ports = append(ports, l)
		}
	}
	return ports
}

func hasPort(outputs bool, prefix string) bool {
	for _, p := range Ports(outputs) {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func Link(a, b string) bool {
	out, err := exec.Command("pw-link", a, b).CombinedOutput()
	return err == nil || strings.Contains(strings.ToLower(string(out)), "exists")
}

func Unlink(a, b string) { _ = exec.Command("pw-link", "-d", a, b).Run() }

var quantumRe = regexp.MustCompile(`value:'(\d+)'`)

func GetForceQuantum() int {
	m := quantumRe.FindStringSubmatch(output("pw-metadata", "-n", "settings", "0", "clock.force-quantum"))
	if m == nil {
		return 0
	}
	q, _ := strconv.Atoi(m[1])
	return q
}

func SetForceQuantum(q int) {
	_ = exec.Command("pw-metadata", "-n", "settings", "0", "clock.force-quantum", strconv.Itoa(q)).Run()
}

func capturePort(source string) string {
	var ports []string
	for _, p := range Ports(true) {
		if strings.HasPrefix(p, source+":capture_") {
			ports = append(ports, p)
		}
	}
	for _, suffix := range []string{"capture_MONO", "capture_FL"} {
		for _, p := range ports {
			if strings.HasSuffix(p, suffix) {
				return p
			}
		}
	}
	if len(ports) > 0 {
		return ports[0]
	}
	return ""
}

func playbackPorts(sink string) (left, right string) {
	for _, p := range Ports(false) {
		if !strings.HasPrefix(p, sink+":playback_") {
			continue
		}
		if left == "" && (strings.HasSuffix(p, "_FL") || strings.HasSuffix(p, "_MONO")) {
			left = p
		}
		if strings.HasSuffix(p, "_FR") {
			right = p
		}
	}
	if right == "" {
		right = left
	}
	return
}

var alsaPrefix = regexp.MustCompile(`^alsa_(input|output)\.`)

// deviceKey: alsa_input.usb-Foo-00.mono-fallback -> usb-Foo-00 (shared with the card's sink).
func deviceKey(name string) string {
	return strings.SplitN(alsaPrefix.ReplaceAllString(name, ""), ".", 2)[0]
}

func pickInput(srcs []Device, wanted string) string {
	for _, s := range srcs {
		if s.Name == wanted {
			return wanted
		}
	}
	for _, s := range srcs {
		if strings.Contains(s.Name, ".usb-") && s.Rate >= 44100 {
			return s.Name
		}
	}
	for _, s := range srcs {
		if strings.HasPrefix(s.Name, "alsa_input.") {
			return s.Name
		}
	}
	if len(srcs) > 0 {
		return srcs[0].Name
	}
	return ""
}

func pickOutput(sinks []Device, input, wanted string) string {
	for _, s := range sinks {
		if s.Name == wanted {
			return wanted
		}
	}
	if input != "" {
		for _, s := range sinks {
			if deviceKey(s.Name) == deviceKey(input) {
				return s.Name
			}
		}
	}
	for _, s := range sinks {
		if strings.Contains(s.Name, "analog") {
			return s.Name
		}
	}
	if len(sinks) > 0 {
		return sinks[0].Name
	}
	return ""
}

func descOf(name string, devs []Device) string {
	for _, d := range devs {
		if d.Name == name {
			return d.Desc
		}
	}
	if name == "" {
		return "none"
	}
	return name
}
