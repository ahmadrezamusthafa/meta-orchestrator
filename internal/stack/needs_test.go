package stack

import (
	"strings"
	"testing"
)

func needsOf(t *testing.T, files map[string]string) string {
	t.Helper()
	var out []string
	for _, n := range Detect(writeRepo(t, files), Deep).Needs() {
		s := n.Tool
		if !n.Required {
			s += "?"
		}
		out = append(out, s)
	}
	return strings.Join(out, ",")
}

func TestNeeds(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		want  string
	}{
		"go service":   {goService, "go,golangci-lint?,node?,zed?,actionlint?,docker?"},
		"react + pnpm": {reactApp, "node,pnpm"},
		"infra":        {infraLive, "tofu,terragrunt"},
		// Wrappers are checked in, so only a JDK is needed.
		"kotlin gradle":         {kotlinService, "java"},
		"java maven":            {javaService, "java"},
		"android":               {androidApp, "java,android-sdk"},
		"flutter":               {flutterApp, "flutter"},
		"dart server":           {dartServer, "dart"},
		"maven without wrapper": {map[string]string{"pom.xml": "<project><properties><java.version>21</java.version></properties></project>"}, "java,maven"},
	}
	for name, c := range cases {
		if got := needsOf(t, c.files); got != c.want {
			t.Errorf("%s: needs = %s, want %s", name, got, c.want)
		}
	}
}
