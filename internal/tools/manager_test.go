package tools

import "testing"

func TestParseVersion(t *testing.T) {
	for in, want := range map[string]string{
		"v20.11.1":                             "20.11.1",
		"Version 1.48.0":                       "1.48.0",
		"Docker version 24.0.6, build ed223bc": "24.0.6",
		"2.0.14 (Claude Code)":                 "2.0.14",
		"git version 2.39.3 (Apple Git-146)":   "2.39.3",
		"ok":                                   "",
	} {
		if got := ParseVersion(in); got != want {
			t.Errorf("ParseVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateVersion(t *testing.T) {
	for _, ok := range []string{"", "1.2.3", "v1.2.3-rc.1", "1.0+build"} {
		if err := ValidateVersion(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"..", "../x", "1/2", `1\2`, "1..2", "-1", " 1"} {
		if ValidateVersion(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	if compareVersions("20.11.1", "18.0.0") <= 0 || compareVersions("18", "18.0.0") != 0 || compareVersions("17.9", "18") >= 0 {
		t.Fatal("compareVersions is wrong")
	}
}
