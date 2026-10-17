package stack

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPackagesListsManifestsPerEcosystem(t *testing.T) {
	root := t.TempDir()
	for rel, content := range map[string]string{
		"package.json":          `{"name":"web"}`,
		"pnpm-lock.yaml":        "lockfileVersion: '9.0'\n",
		"go.mod":                "module x\n",
		"app/pubspec.yaml":      "name: app\ndependencies:\n  flutter:\n    sdk: flutter\n",
		"Gemfile":               "gem 'rails'\n",
		"ml/requirements.txt":   "numpy\n",
		"ml/pyproject.toml":     "[project]\nname='ml'\n",
		"jvm/pom.xml":           "<project/>",
		"jvm/mvnw":              "#!/bin/sh\n",
		"jvm/module/pom.xml":    "<project/>",
		"node_modules/x/go.mod": "module ignored\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := Detect(root, Quick).Packages()
	want := []Package{
		{Ecosystem: "node", Manager: "pnpm", Dir: ".", Manifest: "package.json"},
		{Ecosystem: "go", Manager: "go", Dir: ".", Manifest: "go.mod"},
		{Ecosystem: "flutter", Manager: "flutter", Dir: "app", Manifest: "app/pubspec.yaml"},
		{Ecosystem: "ruby", Manager: "bundler", Dir: ".", Manifest: "Gemfile"},
		{Ecosystem: "python", Manager: "pip", Dir: "ml", Manifest: "ml/pyproject.toml"},
		{Ecosystem: "jvm", Manager: "maven", Dir: "jvm", Manifest: "jvm/pom.xml", Wrapper: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Packages()\n got %+v\nwant %+v", got, want)
	}
}
