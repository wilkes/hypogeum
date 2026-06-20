package main

import (
	"runtime/debug"
	"testing"
)

func buildInfo(mainVer string, settings map[string]string) *debug.BuildInfo {
	info := &debug.BuildInfo{}
	info.Main.Version = mainVer
	for k, v := range settings {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: k, Value: v})
	}
	return info
}

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name                       string
		ldVer, ldCom, ldDate       string
		info                       *debug.BuildInfo
		ok                         bool
		wantVer, wantCom, wantDate string
	}{
		{
			name:  "ldflags win over build info",
			ldVer: "v1.2.3", ldCom: "abc123", ldDate: "2026-06-20",
			info:    buildInfo("v9.9.9", map[string]string{"vcs.revision": "zzz", "vcs.time": "2000-01-01"}),
			ok:      true,
			wantVer: "v1.2.3", wantCom: "abc123", wantDate: "2026-06-20",
		},
		{
			name:  "go install @version fills module version",
			ldVer: "devel", ldCom: "none", ldDate: "unknown",
			info:    buildInfo("v0.5.0", nil),
			ok:      true,
			wantVer: "v0.5.0", wantCom: "none", wantDate: "unknown",
		},
		{
			name:  "local build fills commit and date from vcs",
			ldVer: "devel", ldCom: "none", ldDate: "unknown",
			info:    buildInfo("(devel)", map[string]string{"vcs.revision": "deadbeef", "vcs.time": "2026-06-20T00:00:00Z"}),
			ok:      true,
			wantVer: "devel", wantCom: "deadbeef", wantDate: "2026-06-20T00:00:00Z",
		},
		{
			name:  "dirty working tree marks commit",
			ldVer: "devel", ldCom: "none", ldDate: "unknown",
			info:    buildInfo("(devel)", map[string]string{"vcs.revision": "deadbeef", "vcs.modified": "true"}),
			ok:      true,
			wantVer: "devel", wantCom: "deadbeef-dirty", wantDate: "unknown",
		},
		{
			name:  "no build info leaves defaults",
			ldVer: "devel", ldCom: "none", ldDate: "unknown",
			info:    nil,
			ok:      false,
			wantVer: "devel", wantCom: "none", wantDate: "unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ver, com, dat := resolveVersion(tc.ldVer, tc.ldCom, tc.ldDate, tc.info, tc.ok)
			if ver != tc.wantVer || com != tc.wantCom || dat != tc.wantDate {
				t.Errorf("resolveVersion = (%q, %q, %q), want (%q, %q, %q)",
					ver, com, dat, tc.wantVer, tc.wantCom, tc.wantDate)
			}
		})
	}
}
