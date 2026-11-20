package main

// Runs Guitarix headless and talks to it over its JSON-RPC port.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const rate = 48000

// gxName prefixes the JACK clients (guitarfx_amp, guitarfx_fx). GUITARFX_INSTANCE
// lets a second copy run side by side (handy for testing).
var gxName = envOr("GUITARFX_INSTANCE", "guitarfx")

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type Guitarix struct {
	cmd    *exec.Cmd
	exited chan struct{}
	conn   net.Conn
	rd     *bufio.Reader
	mu     sync.Mutex
	nextID int
	params map[string]bool
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (g *Guitarix) Start(quantum int, logPath string) error {
	for _, tool := range []string{"guitarix", "pw-jack"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s not found (pacman -S guitarix pipewire-jack)", tool)
		}
	}
	port, err := freePort()
	if err != nil {
		return err
	}
	logf, err := os.Create(logPath)
	if err != nil {
		return err
	}
	// -N no GUI, -K don't save Guitarix state, -J we do the wiring ourselves.
	g.cmd = exec.Command("pw-jack", "guitarix", "-N", "-K", "-J", "-p", strconv.Itoa(port), "-n", gxName)
	g.cmd.Env = append(os.Environ(), fmt.Sprintf("PIPEWIRE_LATENCY=%d/%d", quantum, rate))
	g.cmd.Stdout, g.cmd.Stderr = logf, logf
	g.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := g.cmd.Start(); err != nil {
		return err
	}
	g.exited = make(chan struct{})
	go func() { _ = g.cmd.Wait(); logf.Close(); close(g.exited) }()

	deadline := time.Now().Add(20 * time.Second)
	for {
		select {
		case <-g.exited:
			return fmt.Errorf("guitarix exited early, see %s", logPath)
		default:
		}
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			g.conn, g.rd = c, bufio.NewReaderSize(c, 1<<20)
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("guitarix did not start, see %s", logPath)
		}
		time.Sleep(200 * time.Millisecond)
	}
	for !hasPort(false, gxName+"_amp:in_0") {
		if time.Now().After(deadline) {
			return fmt.Errorf("guitarix audio ports did not appear")
		}
		time.Sleep(200 * time.Millisecond)
	}
	return g.loadParams()
}

func (g *Guitarix) Stop() {
	if g.conn != nil {
		g.conn.Close()
		g.conn = nil
	}
	if g.cmd != nil && g.cmd.Process != nil {
		select {
		case <-g.exited:
		default:
			_ = g.cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-g.exited:
			case <-time.After(3 * time.Second):
				_ = g.cmd.Process.Kill()
			}
		}
	}
	g.cmd = nil
}

// clean converts bools to ints: Guitarix's JSON parser does not understand true/false.
func clean(params []any) []any {
	out := make([]any, len(params))
	for i, p := range params {
		if b, ok := p.(bool); ok {
			if b {
				p = 1
			} else {
				p = 0
			}
		}
		out[i] = p
	}
	return out
}

func (g *Guitarix) send(msg map[string]any) error {
	if g.conn == nil {
		return fmt.Errorf("guitarix is not running")
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_ = g.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = g.conn.Write(append(b, '\n'))
	return err
}

func (g *Guitarix) Call(method string, params ...any) (json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nextID++
	id := strconv.Itoa(g.nextID)
	if err := g.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": clean(params), "id": id}); err != nil {
		return nil, err
	}
	_ = g.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		line, err := g.rd.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("guitarix connection lost: %w", err)
		}
		var resp struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(line, &resp) != nil {
			continue
		}
		if string(resp.ID) == strconv.Quote(id) {
			if len(resp.Error) > 0 {
				return nil, fmt.Errorf("guitarix: %s", resp.Error)
			}
			return resp.Result, nil
		}
	}
}

func (g *Guitarix) Notify(method string, params ...any) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": clean(params)})
}

// Set sets parameters and returns ids Guitarix doesn't know (those are never sent,
// since Guitarix can misbehave on bad input).
func (g *Guitarix) Set(vals map[string]any) []string {
	var flat []any
	var bad []string
	for k, v := range vals {
		if !g.params[k] {
			bad = append(bad, k)
			continue
		}
		flat = append(flat, k, v)
	}
	if len(flat) > 0 {
		_ = g.Notify("set", flat...)
	}
	return bad
}

func (g *Guitarix) loadParams() error {
	raw, err := g.Call("parameterlist")
	if err != nil {
		return err
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	g.params = map[string]bool{}
	for i := 1; i < len(list); i += 2 {
		var obj map[string]json.RawMessage
		if json.Unmarshal(list[i], &obj) != nil {
			continue
		}
		var p struct {
			Parameter struct {
				ID string `json:"id"`
			} `json:"Parameter"`
		}
		if pr, ok := obj["Parameter"]; ok {
			_ = json.Unmarshal(pr, &p.Parameter)
		} else {
			for _, v := range obj {
				if json.Unmarshal(v, &p) == nil && p.Parameter.ID != "" {
					break
				}
			}
		}
		if p.Parameter.ID != "" {
			g.params[p.Parameter.ID] = true
		}
	}
	return nil
}

func (g *Guitarix) TunerFreq() float64 {
	raw, err := g.Call("get_tuner_freq")
	if err != nil {
		return 0
	}
	var f float64
	_ = json.Unmarshal(raw, &f)
	return f
}
