package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------- styles

var (
	cAccent = lipgloss.AdaptiveColor{Light: "#7D56F4", Dark: "#9D7CFF"}
	cPink   = lipgloss.AdaptiveColor{Light: "#E0457B", Dark: "#FF5F87"}
	cGreen  = lipgloss.AdaptiveColor{Light: "#02875A", Dark: "#04D98B"}
	cAmber  = lipgloss.AdaptiveColor{Light: "#B26B00", Dark: "#FFB547"}
	cRed    = lipgloss.AdaptiveColor{Light: "#D0153F", Dark: "#FF4F6D"}
	cText   = lipgloss.AdaptiveColor{Light: "#1F1F28", Dark: "#E6E6F0"}
	cMuted  = lipgloss.AdaptiveColor{Light: "#7A7A8C", Dark: "#8A8AA0"}
	cFaint  = lipgloss.AdaptiveColor{Light: "#C8C8D4", Dark: "#3A3A4A"}
	cPillBg = lipgloss.AdaptiveColor{Light: "#ECE8FF", Dark: "#2B2540"}

	sTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(cAccent).Padding(0, 1)
	sMuted  = lipgloss.NewStyle().Foreground(cMuted)
	sText   = lipgloss.NewStyle().Foreground(cText)
	sBold   = lipgloss.NewStyle().Foreground(cText).Bold(true)
	sKey    = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sPanel  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cFaint).Padding(0, 1)
	sHead   = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sPill   = lipgloss.NewStyle().Foreground(cText).Background(cPillBg).Padding(0, 1)
	sAmpPil = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(cPink).Bold(true).Padding(0, 1)
	sSel    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(cAccent).Bold(true)
	sActive = lipgloss.NewStyle().Foreground(cGreen).Bold(true)
	sRec    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(cRed).Bold(true).Padding(0, 1)
	sRecDim = lipgloss.NewStyle().Foreground(cRed).Bold(true).Padding(0, 1)
	sWarn   = lipgloss.NewStyle().Foreground(cRed)
	sOK     = lipgloss.NewStyle().Foreground(cGreen)
	sBadge  = lipgloss.NewStyle().Foreground(lipgloss.Color("#1F1F28")).Background(cAmber).Bold(true).Padding(0, 1)
)

// ---------------------------------------------------------------- model

type item struct {
	song *Song
	part *Part
}

type tickMsg time.Time

type model struct {
	rig    *Rig
	songs  []Song
	items  []item
	sel    int
	tuner  bool
	w, h   int
	freq   float64 // smoothed tuner frequency (0 = no note)
	seenAt time.Time
}

func newModel(rig *Rig, songs []Song, startTuner bool) model {
	m := model{rig: rig, songs: songs, tuner: startTuner}
	for si := range songs {
		for pi := range songs[si].Parts {
			m.items = append(m.items, item{&songs[si], &songs[si].Parts[pi]})
		}
	}
	for i, it := range m.items {
		if len(rig.cfg.Last) == 2 && rig.cfg.Last[0] == it.song.Song && rig.cfg.Last[1] == it.part.Name {
			m.sel = i
		}
	}
	if len(m.items) > 0 {
		rig.Apply(m.items[m.sel].song, m.items[m.sel].part)
	}
	if startTuner {
		rig.Mute(true)
	}
	return m
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Init() tea.Cmd { return tick(100 * time.Millisecond) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	r := m.rig
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		if m.tuner {
			f := r.gx.TunerFreq()
			if f >= tunerMinHz && f <= tunerMaxHz {
				if m.freq == 0 || math.Abs(f-m.freq)/m.freq > 0.03 {
					m.freq = f
				} else {
					m.freq = m.freq*0.6 + f*0.4
				}
				m.seenAt = time.Now()
			} else if time.Since(m.seenAt) > 1500*time.Millisecond {
				m.freq = 0
			}
			return m, tick(80 * time.Millisecond)
		}
		return m, tick(250 * time.Millisecond)
	case tea.KeyMsg:
		k := msg.String()
		if k == "ctrl+c" {
			return m, tea.Quit
		}
		if m.tuner { // any key leaves the tuner
			m.tuner = false
			r.Mute(false)
			return m, nil
		}
		switch k {
		case "q", "esc":
			return m, tea.Quit
		case "up", "k":
			m.sel = (m.sel - 1 + len(m.items)) % len(m.items)
		case "down", "j":
			m.sel = (m.sel + 1) % len(m.items)
		case "enter", " ":
			r.Apply(m.items[m.sel].song, m.items[m.sel].part)
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if i := int(k[0] - '1'); i < len(m.items) {
				m.sel = i
				r.Apply(m.items[i].song, m.items[i].part)
			}
		case "t":
			m.tuner, m.freq = true, 0
			r.Mute(true)
			return m, nil
		case "r":
			r.ToggleRecording()
		case "b":
			r.ToggleBypass()
		case "+", "=":
			r.ChangeVolume(1)
		case "-", "_":
			r.ChangeVolume(-1)
		case "]":
			r.ChangeInputGain(5)
		case "[":
			r.ChangeInputGain(-5)
		case "i":
			r.Cycle(true)
		case "o":
			r.Cycle(false)
		case "l":
			q := r.Quantum()
			for i, v := range quantums {
				if v == q {
					r.SetQuantum(quantums[(i+1)%len(quantums)])
				}
			}
		}
		r.cfg.Save()
	}
	return m, nil
}

// ---------------------------------------------------------------- views

func (m model) View() string {
	if m.w == 0 {
		return ""
	}
	w := min(m.w, 110)
	var body string
	if m.tuner {
		body = m.tunerView(w)
	} else {
		body = m.mainView(w)
	}
	out := lipgloss.JoinVertical(lipgloss.Left, m.header(w), "", body, "", m.footer(w))
	return lipgloss.NewStyle().Padding(1, 2).Render(out)
}

func (m model) header(w int) string {
	left := sTitle.Render("🎸 lazyguitarix") + "  " + sMuted.Render("guitar → USB dongle → Guitarix → headphones")
	right := ""
	if m.rig.Rec.Active() {
		label := "● REC " + fmtDur(m.rig.Rec.Elapsed())
		if time.Now().UnixMilli()/500%2 == 0 {
			right = sRec.Render(label)
		} else {
			right = sRecDim.Render(label)
		}
	}
	gap := max(1, w-4-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

func (m model) footer(w int) string {
	var help string
	if m.tuner {
		help = keyHelp([][2]string{{"any key", "back"}, {"ctrl+c", "quit"}})
	} else {
		help = keyHelp([][2]string{{"↑↓ ⏎", "load"}, {"1-9", "quick"}, {"t", "tuner"}, {"r", "record"},
			{"b", "bypass"}, {"-/+", "volume"}, {"[/]", "gain"}, {"i/o", "devices"}, {"l", "buffer"}, {"q", "quit"}})
	}
	credit := sMuted.Render("made with ") + lipgloss.NewStyle().Foreground(cPink).Render("♥") +
		sMuted.Render(" by Puru · for hobby guitarists")
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Width(w-4).Render(help),
		"",
		lipgloss.PlaceHorizontal(w-4, lipgloss.Center, credit))
}

func keyHelp(keys [][2]string) string {
	var parts []string
	for _, k := range keys {
		parts = append(parts, sKey.Render(k[0])+" "+sMuted.Render(k[1]))
	}
	return strings.Join(parts, sMuted.Render("  ·  "))
}

func (m model) mainView(w int) string {
	r := m.rig
	inner := w - 4
	wide := inner >= 84
	listW := inner
	if wide {
		listW = 40
	}

	// presets list
	var lines []string
	n := 0
	for si, s := range m.songs {
		if si > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, sBold.Render(s.Artist), sMuted.Render(s.Song))
		for pi := range s.Parts {
			p := &m.songs[si].Parts[pi]
			active := r.CurPart == p
			num := fmt.Sprintf(" %d ", n+1)
			name := truncate(p.Name, listW-10)
			marker := "  "
			if active {
				marker = sActive.Render("▶ ")
			}
			row := marker + num + name
			switch {
			case n == m.sel:
				row = marker + sSel.Render(num+name+strings.Repeat(" ", max(0, listW-8-lipgloss.Width(num+name))))
			case active:
				row = marker + sActive.Render(num+name)
			default:
				row = marker + sMuted.Render(num) + sText.Render(name)
			}
			lines = append(lines, row)
			n++
		}
	}
	presets := sPanel.Width(listW - 2).Render(sHead.Render("Presets") + "\n\n" + strings.Join(lines, "\n"))

	// now playing
	npW := inner
	if wide {
		npW = inner - listW - 1
	}
	var np []string
	np = append(np, sHead.Render("Now playing"), "")
	if r.CurPart != nil {
		np = append(np, sBold.Render(r.CurSong.Song)+sMuted.Render("  "+r.CurSong.Album), sActive.Render(r.CurPart.Name), "")
		desc := m.items[m.sel].part.Description
		if m.items[m.sel].part != r.CurPart {
			desc = sMuted.Render("selected: ") + m.items[m.sel].part.Name + " — " + desc
		}
		np = append(np, lipgloss.NewStyle().Foreground(cMuted).Width(npW-6).Render(desc), "")
		np = append(np, pills(chainLabels(r.CurPart), npW-6))
	}
	if r.Bypass {
		np = append(np, "", sBadge.Render("BYPASS · dry guitar"))
	}
	nowPlaying := sPanel.Width(npW - 2).Render(strings.Join(np, "\n"))

	var top string
	if wide {
		h := max(lipgloss.Height(presets), lipgloss.Height(nowPlaying))
		top = lipgloss.JoinHorizontal(lipgloss.Top,
			sPanel.Width(listW-2).Height(h-2).Render(sHead.Render("Presets")+"\n\n"+strings.Join(lines, "\n")), " ",
			sPanel.Width(npW-2).Height(h-2).Render(strings.Join(np, "\n")))
	} else {
		top = lipgloss.JoinVertical(lipgloss.Left, presets, nowPlaying)
	}

	// rig panel
	gain := r.InputGain()
	gainTxt := "?"
	if gain >= 0 {
		gainTxt = fmt.Sprintf("%d%%", gain)
	}
	label := lipgloss.NewStyle().Foreground(cMuted).Width(8)
	rigLines := []string{
		sHead.Render("Rig"), "",
		label.Render("IN") + sText.Render(truncate(descOf(r.Input, r.Sources), inner-30)) + sMuted.Render("   gain ") + sText.Render(gainTxt),
		label.Render("OUT") + sText.Render(truncate(descOf(r.Output, r.Sinks), inner-14)),
		label.Render("VOLUME") + meter(r.VolumeDB(), 24) + sText.Render(fmt.Sprintf("  %+.0f dB", r.VolumeDB())) +
			sMuted.Render("     buffer ") + sText.Render(fmt.Sprintf("%d · %.1f ms", r.Quantum(), r.LatencyMs())),
	}
	if r.Status != "" {
		st := sOK
		if r.StatusBad {
			st = sWarn
		}
		rigLines = append(rigLines, "", st.Width(inner-6).Render(r.Status))
	}
	rigPanel := sPanel.Width(inner - 2).Render(strings.Join(rigLines, "\n"))

	return lipgloss.JoinVertical(lipgloss.Left, top, rigPanel)
}

func pills(labels []string, width int) string {
	var rows []string
	row := ""
	arrow := sMuted.Render(" › ")
	for i, l := range labels {
		st := sPill
		if strings.HasPrefix(l, "amp ") {
			st = sAmpPil
		}
		p := st.Render(l)
		add := p
		if i > 0 && row != "" {
			add = arrow + p
		}
		if row != "" && lipgloss.Width(row)+lipgloss.Width(add) > width {
			rows = append(rows, row)
			row, add = "", sMuted.Render("› ")+p
		}
		row += add
	}
	if row != "" {
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
}

func meter(db float64, width int) string {
	filled := int(math.Round((db + 40) / 40 * float64(width)))
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			c := cGreen
			if i >= width*3/4 {
				c = cAmber
			}
			b.WriteString(lipgloss.NewStyle().Foreground(c).Render("▮"))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(cFaint).Render("▮"))
		}
	}
	return b.String()
}

func truncate(s string, n int) string {
	if n < 4 || lipgloss.Width(s) <= n {
		return s
	}
	rs := []rune(s)
	if len(rs) > n-1 {
		rs = rs[:n-1]
	}
	return string(rs) + "…"
}

func (m model) tunerView(w int) string {
	inner := w - 8
	center := lipgloss.NewStyle().Width(inner).Align(lipgloss.Center)
	var lines []string
	lines = append(lines, sHead.Render("Tuner")+sMuted.Render("   output muted"), "")

	gaugeW := min(51, inner-12)
	half := gaugeW / 2
	if m.freq == 0 {
		lines = append(lines, "", center.Render(sMuted.Render("play a string…")), "", "",
			center.Render(gauge(half, nil, cMuted)), "", "", "")
	} else {
		t := readTuner(m.freq)
		col := cRed
		switch a := abs(t.Cents); {
		case a <= 3:
			col = cGreen
		case a <= 15:
			col = cAmber
		}
		big := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(col).Padding(0, 2).
			Render(noteName(standardTuning[t.StringIx]))
		lines = append(lines,
			center.Render(big),
			center.Render(sBold.Render(stringNames[t.StringIx])),
			center.Render(sMuted.Render(fmt.Sprintf("you're playing %s · %.1f Hz · %+d cents", t.Played, t.Freq, t.Cents))),
			"",
			center.Render(gauge(half, &t.Cents, col)),
			"")
		var msg, hint string
		if abs(t.Cents) <= 3 {
			msg, hint = "✔  IN TUNE — leave it", "move on to the next string"
		} else {
			amount := "a lot"
			if abs(t.Cents) <= 10 {
				amount = "a tiny bit"
			} else if abs(t.Cents) <= 30 {
				amount = "a little"
			}
			if t.Cents < 0 {
				msg, hint = "▲  TIGHTEN "+amount, "too low: turn the peg to make the string tighter (pitch goes up)"
			} else {
				msg, hint = "▼  LOOSEN "+amount, "too high: turn the peg to make the string looser (pitch goes down)"
			}
		}
		lines = append(lines,
			center.Render(lipgloss.NewStyle().Foreground(col).Bold(true).Render(msg)),
			center.Render(sMuted.Render(hint)))
	}

	// string row
	var strs []string
	cur := -1
	if m.freq != 0 {
		cur = readTuner(m.freq).StringIx
	}
	for i, mid := range standardTuning {
		s := lipgloss.NewStyle().Padding(0, 1).Foreground(cMuted)
		if i == cur {
			s = s.Foreground(lipgloss.Color("#FFFFFF")).Background(cAccent).Bold(true)
		}
		strs = append(strs, s.Render(noteName(mid)))
	}
	lines = append(lines, "", "",
		center.Render(sMuted.Render("standard tuning  ")+strings.Join(strs, " ")),
		center.Render(sMuted.Render("6th string (thickest)  →  1st string (thinnest)")))
	return sPanel.Width(w - 6).Render(strings.Join(lines, "\n"))
}

// gauge draws flat ━━━┃━━━ sharp with a ● marker at cents (nil = no marker).
func gauge(half int, cents *int, col lipgloss.TerminalColor) string {
	pos := -1
	if cents != nil {
		c := max(-50, min(50, *cents))
		pos = half + int(math.Round(float64(c)/50*float64(half)))
	}
	var b strings.Builder
	b.WriteString(sMuted.Render("flat  "))
	for i := 0; i <= 2*half; i++ {
		switch {
		case i == pos:
			b.WriteString(lipgloss.NewStyle().Foreground(col).Bold(true).Render("●"))
		case i == half:
			b.WriteString(sBold.Render("┃"))
		case abs(i-half) <= max(1, half*3/50):
			b.WriteString(lipgloss.NewStyle().Foreground(cGreen).Render("━"))
		default:
			b.WriteString(lipgloss.NewStyle().Foreground(cFaint).Render("━"))
		}
	}
	b.WriteString(sMuted.Render("  sharp"))
	return b.String()
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
