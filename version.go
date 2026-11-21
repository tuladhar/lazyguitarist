package main

import (
	"regexp"
	"runtime/debug"
)

const repoURL = "https://github.com/tuladhar/lazyguitarist"

// version is set at release time with -ldflags "-X main.version=v0.1.0".
var version = ""

// pseudo matches the made-up versions Go stamps on untagged builds (v0.0.0-20261121122405-48768cf89d04).
var pseudo = regexp.MustCompile(`\d{14}-[0-9a-f]{12}`)

// Version returns the release tag, the module version from `go install …@vX`, or the git commit
// the binary was built from ("dev" if none of these is known).
func Version() string {
	v, _ := versionInfo()
	return v
}

// VersionURL is the GitHub page for Version(): the release, the commit, or the repo.
func VersionURL() string {
	_, url := versionInfo()
	return url
}

func versionInfo() (label, url string) {
	if version != "" {
		return version, repoURL + "/releases/tag/" + version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev", repoURL
	}
	if v := info.Main.Version; v != "" && v != "(devel)" && !pseudo.MatchString(v) {
		return v, repoURL + "/releases/tag/" + v
	}
	rev, dirty := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "dev", repoURL
	}
	label = rev
	if len(label) > 7 {
		label = label[:7]
	}
	if dirty {
		label += "-dirty"
	}
	return label, repoURL + "/commit/" + rev
}

// hyperlink makes text clickable in terminals that support OSC 8 links (others just show the text).
func hyperlink(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}
