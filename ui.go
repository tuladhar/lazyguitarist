package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	sHead   = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sPill   = lipgloss.NewStyle().Foreground(cText).Background(cPillBg).Padding(0, 1)
	sAmpPil = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(cPink).Bold(true).Padding(0, 1)
	sSel    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(cAccent).Bold(true)
	sSelDim = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sActive = lipgloss.NewStyle().Foreground(cGreen).Bold(true)
	sRec    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(cRed).Bold(true).Padding(0, 1)
	sRecDim = lipgloss.NewStyle().Foreground(cRed).Bold(true).Padding(0, 1)
	sWarn   = lipgloss.NewStyle().Foreground(cRed)
	sBadge  = lipgloss.NewStyle().Foreground(lipgloss.Color("#1F1F28")).Background(cAmber).Bold(true).Padding(0, 1)
)

// ---------------------------------------------------------------- model

type tickMsg time.Time

// focus is the panel that keys act on; its number is the key that focuses it, like lazygit.
const (
	focusMain = iota
	focusSongs
	focusParts
	focusRig
)

// focusOrder is the order tab moves through the panels.
var focusOrder = []int{focusSongs, focusParts, focusRig, focusMain}

// rigRows are the settings in the Rig panel, in display order.
const (
	rigIn = iota
	rigOut
	rigVol
	rigGain
	rigBuffer
	rigRows
)

type model struct {
	rig    *Rig
	songs  []Song
	song   int // cursor in the Songs panel
	part   int // cursor in the Parts panel (a part of songs[song])
	focus  int
	rigRow int // cursor in the Rig panel
	tuner  bool
	help   bool
	w, h   int
	freq   float64 // smoothed tuner frequency (0 = no note)
	seenAt time.Time
}

func newModel(rig *Rig, songs []Song, startTuner bool) model {
	m := model{rig: rig, songs: songs, tuner: startTuner, focus: focusParts}
	for si, s := range songs {
		for pi, p := range s.Parts {
			if len(rig.cfg.Last) == 2 && rig.cfg.Last[0] == s.Song && rig.cfg.Last[1] == p.Name {
				m.song, m.part = si, pi
			}
		}
	}
	if len(songs) > 0 {
		m.load()
	}
	rig.setStatus(false, "Tip: 1 Songs · 2 Parts · 3 Rig · 0 main panel; j/k move, enter plays, ? shows every key")
	if startTuner {
		rig.Mute(true)
	}
	return m
}

func (m *model) curSong() *Song { return &m.songs[m.song] }

// load applies the part under the cursor.
func (m *model) load() {
	s := m.curSong()
	if m.part < len(s.Parts) {
		m.rig.Apply(s, &s.Parts[m.part])
	}
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
		if m.help && k != "?" {
			m.help = false
			if k == "esc" {
				return m, nil
			}
		}
		if len(m.songs) == 0 {
			if k == "q" || k == "esc" {
				return m, tea.Quit
			}
			return m, nil
		}
		switch k {
		case "q", "esc":
			return m, tea.Quit
		case "?":
			m.help = !m.help
		case "0", "1", "2", "3":
			m.focus = int(k[0] - '0')
		case "tab", "right", "l", "shift+tab", "left", "h":
			d := 1
			if k == "shift+tab" || k == "left" || k == "h" {
				d = len(focusOrder) - 1
			}
			for i, f := range focusOrder {
				if f == m.focus {
					m.focus = focusOrder[(i+d)%len(focusOrder)]
					break
				}
			}
		case "up", "k", "down", "j":
			d := 1
			if k == "up" || k == "k" {
				d = -1
			}
			switch {
			case m.focus == focusRig:
				m.rigRow = (m.rigRow + d + rigRows) % rigRows
			case m.focus == focusSongs:
				m.song = (m.song + d + len(m.songs)) % len(m.songs)
				m.part = 0
				for pi := range m.curSong().Parts { // land on the playing part if this song is playing
					if &m.curSong().Parts[pi] == r.CurPart {
						m.part = pi
					}
				}
			case m.focus == focusParts && len(m.curSong().Parts) > 0:
				n := len(m.curSong().Parts)
				m.part = (m.part + d + n) % n
			}
		case "enter", " ":
			if m.focus == focusRig {
				m.adjustRig(1)
			} else {
				m.load()
				m.focus = focusParts
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
			if m.focus == focusRig {
				m.adjustRig(1)
			} else {
				r.ChangeVolume(1)
			}
		case "-", "_":
			if m.focus == focusRig {
				m.adjustRig(-1)
			} else {
				r.ChangeVolume(-1)
			}
		case "]":
			r.ChangeInputGain(5)
		case "[":
			r.ChangeInputGain(-5)
		case "i":
			r.Cycle(true)
		case "o":
			r.Cycle(false)
		case "L":
			nextQuantum(r, 1)
		}
		r.cfg.Save()
	}
	return m, nil
}

// adjustRig changes the Rig setting under the cursor up (d = 1) or down (d = -1).
func (m *model) adjustRig(d int) {
	r := m.rig
	switch m.rigRow {
	case rigIn:
		r.Cycle(true)
	case rigOut:
		r.Cycle(false)
	case rigVol:
		r.ChangeVolume(float64(d))
	case rigGain:
		r.ChangeInputGain(5 * d)
	case rigBuffer:
		nextQuantum(r, d)
	}
}

func nextQuantum(r *Rig, d int) {
	q := r.Quantum()
	for i, v := range quantums {
		if v == q {
			r.SetQuantum(quantums[(i+d+len(quantums))%len(quantums)])
			return
		}
	}
}

// ---------------------------------------------------------------- layout

// View fills the whole terminal, lazygit-style: Songs, Parts and Rig on the left, the main panel (part details,
// tuner or help) and the Log on the right, and a one-line key bar at the bottom.
func (m model) View() string {
	if m.w < 20 || m.h < 8 {
		return "lazyguitarist: make the terminal bigger"
	}
	bodyH := m.h - 1
	var body string
	if m.w >= 90 {
		leftW := max(32, min(48, m.w*36/100))
		rightW := m.w - leftW
		rig := m.rigPanel(leftW)
		rest := bodyH - lipgloss.Height(rig)
		songsH := max(4, rest/2)
		partsH := max(4, rest-songsH)
		left := lipgloss.JoinVertical(lipgloss.Left,
			m.songsPanel(leftW, songsH), m.partsPanel(leftW, partsH), rig)

		logH := max(5, min(10, bodyH/4))
		right := lipgloss.JoinVertical(lipgloss.Left,
			m.mainPanel(rightW, bodyH-logH), m.logPanel(rightW, logH))
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	} else {
		// narrow: one column, no Log
		rig := m.rigPanel(m.w)
		songsH := min(len(m.songs)+2, 7)
		partsH := 4
		if len(m.songs) > 0 {
			partsH = min(len(m.curSong().Parts)+2, 8)
		}
		mainH := max(5, bodyH-lipgloss.Height(rig)-songsH-partsH)
		body = lipgloss.JoinVertical(lipgloss.Left,
			m.songsPanel(m.w, songsH), m.partsPanel(m.w, partsH), m.mainPanel(m.w, mainH), rig)
	}
	// never taller than the terminal, or it drops the top lines
	lines := strings.Split(body, "\n")
	if len(lines) > bodyH {
		lines = lines[:bodyH]
	}
	return strings.Join(lines, "\n") + "\n" + m.statusBar()
}

// panel draws a box of exactly w×h with the title in the top border (lazygit-style) and an optional note, such
// as a "2 of 5" position, in the bottom border.
func panel(title, note string, body []string, w, h int, focused bool) string {
	w, h = max(w, 6), max(h, 2)
	bc := lipgloss.NewStyle().Foreground(cFaint)
	tc := sText
	if focused {
		bc = lipgloss.NewStyle().Foreground(cGreen)
		tc = sActive
	}
	inner := w - 2
	t := ansi.Truncate(tc.Render(title), inner-2, "…")
	top := bc.Render("╭─") + t + bc.Render(strings.Repeat("─", max(0, inner-1-lipgloss.Width(t)))+"╮")
	n := ansi.Truncate(note, inner-2, "")
	bottom := bc.Render("╰"+strings.Repeat("─", max(0, inner-1-lipgloss.Width(n)))) + sMuted.Render(n) + bc.Render("─╯")

	out := []string{top}
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		line = ansi.Truncate(" "+line, inner, "…")
		out = append(out, bc.Render("│")+line+strings.Repeat(" ", max(0, inner-lipgloss.Width(line)))+bc.Render("│"))
	}
	return strings.Join(append(out, bottom), "\n")
}

// window returns the slice [start, end) of n rows that fits in rows lines and keeps sel visible.
func window(n, sel, rows int) (int, int) {
	rows = max(rows, 1)
	if n <= rows {
		return 0, n
	}
	start := max(0, min(sel-rows/2, n-rows))
	return start, start + rows
}

func (m model) songsPanel(w, h int) string {
	var rows []string
	for si, s := range m.songs {
		playing := m.rig.CurSong == &m.songs[si]
		marker := "  "
		if playing {
			marker = sActive.Render("▶ ")
		}
		name := s.Artist + " – " + s.Song
		switch {
		case si == m.song && m.focus == focusSongs:
			rows = append(rows, marker+sSel.Render(" "+name+strings.Repeat(" ", max(0, w-7-lipgloss.Width(name)))))
		case si == m.song:
			rows = append(rows, marker+sSelDim.Render(" "+name))
		case playing:
			rows = append(rows, marker+sActive.Render(" "+name))
		default:
			rows = append(rows, marker+sText.Render(" "+name))
		}
	}
	a, b := window(len(rows), m.song, h-2)
	note := ""
	if len(m.songs) > 0 {
		note = fmt.Sprintf("%d of %d", m.song+1, len(m.songs))
	}
	return panel("[1] Songs", note, rows[a:b], w, h, m.focus == focusSongs)
}

func (m model) partsPanel(w, h int) string {
	if len(m.songs) == 0 {
		return panel("[2] Parts", "", []string{sMuted.Render("no presets found")}, w, h, false)
	}
	s := m.curSong()
	var rows []string
	for pi := range s.Parts {
		p := &s.Parts[pi]
		playing := m.rig.CurPart == p
		marker := "  "
		if playing {
			marker = sActive.Render("▶ ")
		}
		num := fmt.Sprintf("%d ", pi+1)
		switch {
		case pi == m.part && m.focus == focusParts:
			rows = append(rows, marker+sSel.Render(" "+num+p.Name+strings.Repeat(" ", max(0, w-7-lipgloss.Width(num+p.Name)))))
		case pi == m.part:
			rows = append(rows, marker+sSelDim.Render(" "+num+p.Name))
		case playing:
			rows = append(rows, marker+sActive.Render(" "+num+p.Name))
		default:
			rows = append(rows, marker+" "+sMuted.Render(num)+sText.Render(p.Name))
		}
	}
	a, b := window(len(rows), m.part, h-2)
	return panel("[2] Parts", fmt.Sprintf("%d of %d", m.part+1, len(s.Parts)), rows[a:b], w, h, m.focus == focusParts)
}

func (m model) rigPanel(w int) string {
	r := m.rig
	inner := w - 4
	label := lipgloss.NewStyle().Foreground(cMuted).Width(6)
	gain := r.InputGain()
	gainTxt := "?"
	if gain >= 0 {
		gainTxt = fmt.Sprintf("%d%%", gain)
	}
	meterW := max(6, min(24, inner-16))
	focused := m.focus == focusRig
	row := func(i int, name, value string) string {
		if focused && i == m.rigRow {
			return sSel.Render(fmt.Sprintf("%-6s", name)) + value
		}
		return label.Render(name) + value
	}
	body := []string{
		row(rigIn, "IN", sText.Render(truncate(descOf(r.Input, r.Sources), inner-6))),
		row(rigOut, "OUT", sText.Render(truncate(descOf(r.Output, r.Sinks), inner-6))),
		row(rigVol, "VOL", meter(r.VolumeDB(), meterW)+sText.Render(fmt.Sprintf(" %+.0f dB", r.VolumeDB()))),
		row(rigGain, "GAIN", sText.Render(gainTxt)),
		row(rigBuffer, "BUF", sText.Render(fmt.Sprintf("%d · %.1f ms", r.Quantum(), r.LatencyMs()))),
	}
	note := ""
	if focused {
		note = "-/+ change"
	}
	return panel("[3] Rig", note, body, w, len(body)+2, focused)
}

// mainPanel shows the tuner, the key list, or the part under the cursor.
func (m model) mainPanel(w, h int) string {
	inner := w - 4
	switch {
	case m.tuner:
		return panel("[0] Tuner", "any key: back", m.tunerLines(inner), w, h, true)
	case m.help:
		return panel("[0] Keys", "any key: back", helpLines(), w, h, true)
	case len(m.songs) == 0:
		return panel("[0] Now playing", "", []string{sMuted.Render("Add songs to presets.json")}, w, h, false)
	}
	r := m.rig
	s := m.curSong()
	p := &s.Parts[m.part]
	title := "[0] Now playing"
	var body []string
	head := sBold.Render(s.Song) + sMuted.Render("  "+s.Artist)
	if s.Album != "" {
		head += sMuted.Render(" · " + s.Album)
	}
	body = append(body, head)
	if p == r.CurPart {
		body = append(body, sActive.Render(fmt.Sprintf("▶ %d %s", m.part+1, p.Name)))
	} else {
		title = "[0] Part"
		body = append(body, sSelDim.Render(fmt.Sprintf("%d %s", m.part+1, p.Name))+sMuted.Render("   enter to play"))
	}
	body = append(body, "")
	body = append(body, strings.Split(lipgloss.NewStyle().Foreground(cMuted).Width(inner).Render(p.Description), "\n")...)
	body = append(body, "")
	body = append(body, strings.Split(pills(chainLabels(p), inner), "\n")...)
	if g := p.Guitar; g != nil {
		body = append(append(body, ""), guitarView(g)...)
	}
	if r.Bypass {
		body = append(body, "", sBadge.Render("BYPASS · dry guitar"))
	}
	return panel(title, "", body, w, h, m.focus == focusMain)
}

func (m model) logPanel(w, h int) string {
	var rows []string
	for _, l := range m.rig.Log {
		st := sText
		if l.Bad {
			st = sWarn
		} else if strings.HasPrefix(l.Text, "Tip:") {
			st = sMuted
		}
		rows = append(rows, sMuted.Render(l.At.Format("15:04"))+" "+st.Render(l.Text))
	}
	if n := h - 2; len(rows) > n {
		rows = rows[len(rows)-n:]
	}
	return panel("Log", "", rows, w, h, false)
}

// statusBar is the bottom line: key hints on the left; recording, bypass and the name on the right.
func (m model) statusBar() string {
	var left string
	if m.tuner || m.help {
		left = keyHelp([][2]string{{"any key", "back"}, {"ctrl+c", "quit"}})
	} else {
		left = keyHelp([][2]string{{"1-3", "panel"}, {"↑↓", "move"}, {"⏎", "play"}, {"t", "tuner"},
			{"r", "record"}, {"b", "bypass"}, {"-/+", "volume"}, {"?", "keys"}, {"q", "quit"}})
	}
	var right []string
	if m.rig.Rec.Active() {
		label := "● REC " + fmtDur(m.rig.Rec.Elapsed())
		if time.Now().UnixMilli()/500%2 == 0 {
			right = append(right, sRec.Render(label))
		} else {
			right = append(right, sRecDim.Render(label))
		}
	}
	if m.rig.Bypass {
		right = append(right, sBadge.Render("BYPASS"))
	}
	right = append(right, sTitle.Render("🎸 lazyguitarist"))
	r := strings.Join(right, " ")
	left = ansi.Truncate(" "+left, max(0, m.w-lipgloss.Width(r)-1), "…")
	return left + strings.Repeat(" ", max(1, m.w-lipgloss.Width(left)-lipgloss.Width(r))) + r
}

func keyHelp(keys [][2]string) string {
	var parts []string
	for _, k := range keys {
		parts = append(parts, sKey.Render(k[0])+" "+sMuted.Render(k[1]))
	}
	return strings.Join(parts, sMuted.Render(" · "))
}

func helpLines() []string {
	keys := [][2]string{
		{"1  2  3  0", "focus Songs, Parts, Rig, main panel"},
		{"tab  ← →  h l", "next / previous panel"},
		{"↑ ↓  j k", "move in the focused panel"},
		{"enter  space", "play the part under the cursor (Rig: change the setting)"},
		{"- +", "output volume (Rig: change the setting under the cursor)"},
		{"t", "tuner (mutes the output)"},
		{"r", "start / stop recording (saved to ~/Music/lazyguitarist)"},
		{"b", "bypass: hear the dry guitar"},
		{"[ ]", "input gain of the guitar dongle"},
		{"i  o", "next input / output device"},
		{"L", "buffer size (latency)"},
		{"?", "this list"},
		{"q  esc", "quit"},
	}
	var out []string
	for _, k := range keys {
		out = append(out, sKey.Width(16).Render(k[0])+sText.Render(k[1]))
	}
	return out
}

// ---------------------------------------------------------------- widgets

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

// tunerLines draws the tuner for a panel width inner.
func (m model) tunerLines(inner int) []string {
	center := lipgloss.NewStyle().Width(inner).Align(lipgloss.Center)
	var lines []string
	lines = append(lines, sHead.Render("Tuner")+sMuted.Render("   output muted"), "")

	gaugeW := max(11, min(51, inner-12))
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
	return lines
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

// guitarView shows how to set the guitar for a part: the 5-way pickup selector and the three knobs.
func guitarView(g *Guitar) []string {
	label := lipgloss.NewStyle().Foreground(cMuted).Width(9)
	sel := pickupIndex(g.Pickup)
	var dots []string
	for i := range pickupPositions {
		if i == sel {
			dots = append(dots, sActive.Render("●"))
		} else {
			dots = append(dots, lipgloss.NewStyle().Foreground(cFaint).Render("○"))
		}
	}
	knob := func(v float64) string {
		n := max(0, min(10, int(math.Round(v))))
		return sKey.Render(strings.Repeat("▮", n)) + lipgloss.NewStyle().Foreground(cFaint).Render(strings.Repeat("▮", 10-n)) +
			sText.Render(fmt.Sprintf(" %g", v))
	}
	return []string{
		sHead.Render("Guitar"),
		label.Render("PICKUP") + sMuted.Render("bridge ") + strings.Join(dots, " ") + sMuted.Render(" neck   ") + sBold.Render(g.Pickup),
		label.Render("VOLUME") + knob(g.Volume),
		label.Render("TONE 1") + knob(g.Tone1) + sMuted.Render("  neck"),
		label.Render("TONE 2") + knob(g.Tone2) + sMuted.Render("  middle · bridge"),
	}
}
