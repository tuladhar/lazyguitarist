package main

import (
	"regexp"
	"runtime/debug"
)

// pseudo matches the made-up versions Go stamps on untagged builds (v0.0.0-20261121122405-48768cf89d04).
var pseudo = regexp.MustCompile(`\d{14}-[0-9a-f]{12}`)

// version is set at release time with -ldflags "-X main.version=v0.1.0".
var version = ""

// Version returns the release tag, the module version from `go install …@vX`, or the git commit
// the binary was built from ("dev" if none of these is known).
func Version() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" && !pseudo.MatchString(v) {
		return v
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
		return "dev"
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}
