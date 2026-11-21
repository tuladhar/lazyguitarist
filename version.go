package main

import (
	"regexp"
	"runtime/debug"
	"time"
)

const (
	repoURL   = "https://github.com/tuladhar/lazyguitarist"
	author    = "Puru Tuladhar"
	authorURL = "https://github.com/tuladhar"
)

// version is set at release time with -ldflags "-X main.version=v0.1.0".
var version = ""

// pseudo matches the made-up versions Go stamps on untagged builds (v0.0.0-20261121122405-48768cf89d04).
var pseudo = regexp.MustCompile(`\d{14}-[0-9a-f]{12}`)

// BuildInfo says what this binary was built from.
type BuildInfo struct {
	Version string // release tag, or the short commit, or "dev"
	URL     string // GitHub page for Version: the release, the commit, or the repo
	Commit  string // full git commit, if known
	Time    string // commit date, if known
	Dirty   bool   // built with uncommitted changes
}

func Build() BuildInfo {
	b := BuildInfo{Version: "dev", URL: repoURL}
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				b.Commit = s.Value
			case "vcs.modified":
				b.Dirty = s.Value == "true"
			case "vcs.time":
				if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
					b.Time = t.Local().Format("2006-01-02 15:04")
				}
			}
		}
	}
	tag := version
	if tag == "" && ok {
		if v := info.Main.Version; v != "" && v != "(devel)" && !pseudo.MatchString(v) {
			tag = v
		}
	}
	switch {
	case tag != "":
		b.Version, b.URL = tag, repoURL+"/releases/tag/"+tag
	case b.Commit != "":
		b.Version, b.URL = shortRev(b.Commit), repoURL+"/commit/"+b.Commit
		if b.Dirty {
			b.Version += "-dirty"
		}
	}
	return b
}

// Version returns the release tag, the git commit the binary was built from, or "dev".
func Version() string { return Build().Version }

// VersionURL is the GitHub page for Version().
func VersionURL() string { return Build().URL }

func shortRev(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}

// hyperlink makes text clickable in terminals that support OSC 8 links (others just show the text).
func hyperlink(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}
