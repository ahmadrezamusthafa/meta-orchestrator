package packages

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var semverRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?(?:-[0-9A-Za-z.-]+)?`)

// parseVersion extracts the first version number from a command's version output.
func parseVersion(out string) string {
	return semverRe.FindString(out)
}

// isPrerelease reports a version with a pre-release suffix (1.2.0-beta.1).
func isPrerelease(v string) bool {
	core, _, _ := strings.Cut(v, "+")
	return strings.Contains(core, "-")
}

// compareVersions compares dotted versions numerically; a pre-release sorts before its release.
func compareVersions(a, b string) int {
	a, b = strings.TrimPrefix(a, "v"), strings.TrimPrefix(b, "v")
	ca, pa, _ := strings.Cut(strings.SplitN(a, "+", 2)[0], "-")
	cb, pb, _ := strings.Cut(strings.SplitN(b, "+", 2)[0], "-")
	xa, xb := strings.Split(ca, "."), strings.Split(cb, ".")
	for i := 0; i < len(xa) || i < len(xb); i++ {
		var x, y int
		if i < len(xa) {
			x, _ = strconv.Atoi(xa[i])
		}
		if i < len(xb) {
			y, _ = strconv.Atoi(xb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case pa == pb:
		return 0
	case pa == "":
		return 1
	case pb == "":
		return -1
	case pa < pb:
		return -1
	}
	return 1
}

// parseNPMVersions reads `npm view <pkg> versions --json`, which prints a list, or a single
// string when the package has one version. Anything before the JSON (wrapper banners) is skipped.
func parseNPMVersions(out string) ([]string, error) {
	i := strings.IndexAny(out, "[\"")
	if i < 0 {
		return nil, errors.New("npm printed no version list")
	}
	dec := json.NewDecoder(strings.NewReader(out[i:]))
	var raw interface{}
	if err := dec.Decode(&raw); err != nil {
		return nil, errors.New("npm printed an unreadable version list")
	}
	var list []string
	add := func(v interface{}) {
		if s, ok := v.(string); ok && ValidateVersion(s) == nil {
			list = append(list, s)
		}
	}
	switch v := raw.(type) {
	case []interface{}:
		for _, x := range v {
			add(x)
		}
	default:
		add(v)
	}
	return list, nil
}

// parseBrewStable reads the stable version from `brew info --json=v2 <formula>`.
func parseBrewStable(out string) string {
	var info struct {
		Formulae []struct {
			Versions struct {
				Stable string `json:"stable"`
			} `json:"versions"`
		} `json:"formulae"`
	}
	if err := json.Unmarshal([]byte(out), &info); err != nil || len(info.Formulae) == 0 {
		return ""
	}
	v := info.Formulae[0].Versions.Stable
	if ValidateVersion(v) != nil {
		return ""
	}
	return v
}
