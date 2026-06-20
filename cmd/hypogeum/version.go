package main

import (
	"fmt"
	"runtime/debug"
)

// versionLine renders the string printed by `hypogeum --version`.
//
// Precedence: ldflags injected by a GoReleaser release build win. When they're
// still at their source defaults (a plain `go install` or local `go build`,
// which GoReleaser's -X flags never touch), we fall back to the metadata the Go
// toolchain embeds automatically: the module version for `...@latest`/`@vX.Y.Z`
// installs, and the vcs.* build settings for builds from a git working tree.
func versionLine() string {
	info, ok := debug.ReadBuildInfo()
	v, c, d := resolveVersion(version, commit, date, info, ok)
	return fmt.Sprintf("hypogeum %s (commit %s, built %s)", v, c, d)
}

// resolveVersion is the pure core of versionLine, split out so it can be tested
// with a synthesized *debug.BuildInfo instead of the live process's.
func resolveVersion(ldVersion, ldCommit, ldDate string, info *debug.BuildInfo, ok bool) (ver, com, dat string) {
	ver, com, dat = ldVersion, ldCommit, ldDate
	if !ok || info == nil {
		return ver, com, dat
	}

	// Module version: present for `go install pkg@version`. A build from a
	// local source tree reports "(devel)", which is no better than our own
	// "devel" default, so we ignore it and let the vcs.* settings speak.
	if ver == "devel" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		ver = info.Main.Version
	}

	var modified bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if com == "none" && s.Value != "" {
				com = s.Value
			}
		case "vcs.time":
			if dat == "unknown" && s.Value != "" {
				dat = s.Value
			}
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if modified && com != "none" {
		com += "-dirty"
	}
	return ver, com, dat
}
