package main

// Records Guitarix's processed stereo output to a WAV file.
//
// Kept clean on purpose: it taps Guitarix's output ports directly (no mic, no system mix),
// at the graph's own 48 kHz (no resampling), as 32-bit PCM (no 16-bit grit), with a roomy
// 200 ms recorder buffer so slow disk writes can't cause dropouts, and short fades at both
// ends so starting and stopping don't click.

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const recFadeMs = 15

var recNode = gxName + "_rec"

type Recorder struct {
	cmd     *exec.Cmd
	done    chan struct{}
	Path    string
	started time.Time
}

func (r *Recorder) Active() bool { return r.cmd != nil }

func (r *Recorder) Elapsed() time.Duration {
	if !r.Active() {
		return 0
	}
	return time.Since(r.started)
}

var slugRe = regexp.MustCompile(`[^A-Za-z0-9]+`)

func (r *Recorder) Start(dir, label, left, right string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	slug := strings.Trim(slugRe.ReplaceAllString(label, "-"), "-")
	if len(slug) > 60 {
		slug = slug[:60]
	}
	if slug == "" {
		slug = "take"
	}
	r.Path = filepath.Join(dir, time.Now().Format("2006-01-02_15-04-05_")+slug+".wav")
	r.cmd = exec.Command("pw-record",
		"-P", "{ node.autoconnect=false node.name="+recNode+" }",
		"--rate", strconv.Itoa(rate), "--channels", "2", "--format", "s32", "--latency", "200ms",
		r.Path)
	r.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := r.cmd.Start(); err != nil {
		r.cmd = nil
		return err
	}
	r.done = make(chan struct{})
	go func(c *exec.Cmd, done chan struct{}) { _ = c.Wait(); close(done) }(r.cmd, r.done)

	deadline := time.Now().Add(3 * time.Second)
	for !hasPort(false, recNode+":input_FR") {
		select {
		case <-r.done:
			r.cmd = nil
			return fmt.Errorf("pw-record exited")
		default:
		}
		if time.Now().After(deadline) {
			_ = r.cmd.Process.Kill()
			r.cmd = nil
			return fmt.Errorf("pw-record did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	Link(left, recNode+":input_FL")
	Link(right, recNode+":input_FR")
	r.started = time.Now()
	return nil
}

// Stop finalizes the file and returns its path and length.
func (r *Recorder) Stop() (string, time.Duration) {
	if !r.Active() {
		return "", 0
	}
	d := r.Elapsed()
	_ = r.cmd.Process.Signal(os.Interrupt) // pw-record writes the WAV header on SIGINT
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
		_ = r.cmd.Process.Kill()
		<-r.done
	}
	r.cmd = nil
	_ = fadeWavEdges(r.Path, recFadeMs)
	return r.Path, d
}

// fadeWavEdges applies a short linear fade-in/out in place to a 32-bit PCM WAV.
func fadeWavEdges(path string, ms int) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr := make([]byte, 12)
	if _, err := io.ReadFull(f, hdr); err != nil || string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return fmt.Errorf("not a WAV file")
	}
	var channels, bits, srate int
	var dataStart, dataSize int64
	for {
		ch := make([]byte, 8)
		if _, err := io.ReadFull(f, ch); err != nil {
			return fmt.Errorf("no data chunk")
		}
		id, size := string(ch[:4]), int64(binary.LittleEndian.Uint32(ch[4:]))
		if id == "fmt " {
			fmtb := make([]byte, size)
			if _, err := io.ReadFull(f, fmtb); err != nil {
				return err
			}
			channels = int(binary.LittleEndian.Uint16(fmtb[2:4]))
			srate = int(binary.LittleEndian.Uint32(fmtb[4:8]))
			bits = int(binary.LittleEndian.Uint16(fmtb[14:16]))
			if size%2 == 1 {
				_, _ = f.Seek(1, io.SeekCurrent)
			}
		} else if id == "data" {
			dataStart, _ = f.Seek(0, io.SeekCurrent)
			dataSize = size
			break
		} else {
			_, _ = f.Seek(size+size%2, io.SeekCurrent)
		}
	}
	if bits != 32 || channels == 0 {
		return fmt.Errorf("expected 32-bit PCM")
	}
	end, _ := f.Seek(0, io.SeekEnd)
	if dataSize == 0 || dataSize == 0xFFFFFFFF || dataStart+dataSize > end {
		dataSize = end - dataStart
	}
	frameBytes := int64(4 * channels)
	frames := dataSize / frameBytes
	n := int64(srate * ms / 1000)
	if n > frames/2 {
		n = frames / 2
	}
	if n <= 0 {
		return nil
	}
	buf := make([]byte, n*frameBytes)
	for _, fadeIn := range []bool{true, false} {
		off := dataStart
		if !fadeIn {
			off = dataStart + (frames-n)*frameBytes
		}
		if _, err := f.ReadAt(buf, off); err != nil {
			return err
		}
		for i := int64(0); i < n; i++ {
			g := float64(i) / float64(n)
			if !fadeIn {
				g = float64(n-1-i) / float64(n)
			}
			for c := int64(0); c < int64(channels); c++ {
				p := (i*int64(channels) + c) * 4
				v := int32(binary.LittleEndian.Uint32(buf[p:]))
				binary.LittleEndian.PutUint32(buf[p:], uint32(int32(float64(v)*g)))
			}
		}
		if _, err := f.WriteAt(buf, off); err != nil {
			return err
		}
	}
	return nil
}

func fmtDur(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
