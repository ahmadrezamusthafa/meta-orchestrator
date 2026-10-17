package stack

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// kotlinService is a Spring Boot service in Kotlin: a multi-module Gradle build with a wrapper.
var kotlinService = map[string]string{
	"settings.gradle.kts": `rootProject.name = "ledger"
include("api", "core")`,
	"build.gradle.kts": `plugins { kotlin("jvm") version "2.0.0"; id("org.springframework.boot") version "3.3.0" }`,
	"gradlew":          "#!/bin/sh",
	"api/build.gradle.kts": `dependencies {
	implementation("org.springframework.boot:spring-boot-starter-oauth2-resource-server")
	runtimeOnly("org.postgresql:postgresql")
	implementation("io.opentelemetry:opentelemetry-api")
}`,
	"core/build.gradle.kts": `dependencies { implementation("com.authzed.api:authzed:1.0.0") }`,
}

// javaService is a Maven Java service with a wrapper and a child module.
var javaService = map[string]string{
	"pom.xml":       "<project><properties><java.version>21</java.version></properties><modules><module>web</module></modules></project>",
	"mvnw":          "#!/bin/sh",
	"web/pom.xml":   "<project><artifactId>web</artifactId></project>",
	"web/Main.java": "class Main {}",
	"README.md":     "service",
}

// androidApp is a Kotlin Android app using a Gradle version catalog (no plugin ids in build files).
var androidApp = map[string]string{
	"settings.gradle.kts":         `include(":app")`,
	"gradlew":                     "#!/bin/sh",
	"gradle/libs.versions.toml":   "[plugins]\nandroid-application = { id = \"com.android.application\" }\nkotlin-android = { id = \"org.jetbrains.kotlin.android\" }",
	"app/build.gradle.kts":        "plugins { alias(libs.plugins.android.application) }\nandroid {\n  namespace = \"com.example\"\n}",
	"app/src/AndroidManifest.xml": "<manifest/>",
}

// flutterApp is a Flutter app with generated native host projects, code generation and tests.
var flutterApp = map[string]string{
	"pubspec.yaml": `name: wallet
dependencies:
  flutter:
    sdk: flutter
  flutter_appauth: ^6.0.0
dev_dependencies:
  build_runner: ^2.4.0`,
	"lib/main.dart":                       "void main() {}",
	"test/widget_test.dart":               "void main() {}",
	"android/build.gradle":                "android {\n}\napply plugin: 'kotlin-android'",
	"android/app/build.gradle":            "apply plugin: 'com.android.application'",
	"android/settings.gradle":             "include ':app'",
	"android/gradlew":                     "#!/bin/sh",
	"ios/Runner/AppDelegate.swift":        "",
	"android/app/src/AndroidManifest.xml": "<manifest/>",
}

// dartServer is a plain Dart backend (no Flutter) without tests.
var dartServer = map[string]string{
	"pubspec.yaml":    "name: api\ndependencies:\n  shelf: ^1.4.0\n  postgres: ^3.0.0",
	"bin/server.dart": "void main() {}",
}

func TestDetectJVM(t *testing.T) {
	k := Detect(writeRepo(t, kotlinService), Deep)
	if got, want := ids(k), []string{"postgresql", "kotlin", "keycloak", "spicedb", "opentelemetry"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("kotlin service techs = %v, want %v", got, want)
	}
	if len(k.jvmBuilds) != 1 || k.jvmBuilds[0].dir != "." || !k.jvmBuilds[0].gradle || !k.jvmBuilds[0].wrapper {
		t.Fatalf("a multi-module Gradle build runs once from its root with the wrapper: %+v", k.jvmBuilds)
	}
	j := Detect(writeRepo(t, javaService), Deep)
	if got, want := ids(j), []string{"java"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("java service techs = %v, want %v", got, want)
	}
	if len(j.jvmBuilds) != 1 || j.jvmBuilds[0].gradle || !j.jvmBuilds[0].wrapper {
		t.Fatalf("maven parent only, with mvnw: %+v", j.jvmBuilds)
	}
	a := Detect(writeRepo(t, androidApp), Deep)
	if got, want := ids(a), []string{"kotlin", "android"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("android app techs = %v, want %v (version catalogs must be understood)", got, want)
	}
}

func TestDetectFlutterIgnoresNativeScaffolding(t *testing.T) {
	p := Detect(writeRepo(t, flutterApp), Deep)
	if got, want := ids(p), []string{"flutter", "dart", "keycloak"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("flutter techs = %v, want %v — android/ and ios/ belong to the Flutter app", got, want)
	}
	if len(p.jvmBuilds) != 0 {
		t.Errorf("the generated android/ Gradle build is not a separate JVM build: %+v", p.jvmBuilds)
	}
	if len(p.dartPkgs) != 1 || !p.dartPkgs[0].flutter || !p.dartPkgs[0].tests || !p.dartPkgs[0].buildRunner {
		t.Errorf("dart package facts = %+v", p.dartPkgs)
	}
	if s := p.Surface(); s != "mobile" {
		t.Errorf("flutter surface = %q, want mobile", s)
	}
}

func TestClassifyJVMAndMobile(t *testing.T) {
	cases := []struct {
		name     string
		files    map[string]string
		role     types.ProjectRole
		manifest string
	}{
		{"ledger", kotlinService, types.RoleBackend, "build.gradle.kts"},
		{"payments", javaService, types.RoleBackend, "pom.xml"},
		{"wallet-android", androidApp, types.RoleFrontend, "settings.gradle.kts"},
		{"wallet", flutterApp, types.RoleFrontend, "pubspec.yaml"},
		{"dart-api", dartServer, types.RoleBackend, "pubspec.yaml"},
	}
	for _, c := range cases {
		got := Classify(c.name, writeRepo(t, c.files))
		if got.Role != c.role || got.Manifest != c.manifest {
			t.Errorf("%s: got %s/%s, want %s/%s", c.name, got.Role, got.Manifest, c.role, c.manifest)
		}
	}
	surfaces := map[string]map[string]string{"api": kotlinService, "mobile": androidApp}
	for want, files := range surfaces {
		if s := Detect(writeRepo(t, files), Deep).Surface(); s != want {
			t.Errorf("surface = %q, want %q", s, want)
		}
	}
	if s := Detect(writeRepo(t, dartServer), Deep).Surface(); s != "api" {
		t.Errorf("a Dart server is checked through its API, got %q", s)
	}
}

func TestBriefJVMAndMobile(t *testing.T) {
	brief := Brief([]Repo{
		{Name: "ledger", Profile: Detect(writeRepo(t, kotlinService), Deep)},
		{Name: "payments", Profile: Detect(writeRepo(t, javaService), Deep)},
		{Name: "wallet", Profile: Detect(writeRepo(t, flutterApp), Deep)},
		{Name: "api", Profile: Detect(writeRepo(t, dartServer), Deep)},
	})
	for _, want := range []string{
		"- ledger: `./gradlew check` — narrow with `--tests <Class>` while iterating",
		"- payments: `./mvnw -q verify`",
		"- wallet: `dart run build_runner build --delete-conflicting-outputs`",
		"- wallet: `dart format --output=none --set-exit-if-changed .`",
		"- wallet: `flutter analyze`",
		"- wallet: `flutter test`",
		"- api: `dart analyze`",
		"Kotlin: null safety without `!!`",
		"Flutter: small widgets with const constructors",
		"`mvn deploy`, `gradle publish`",
		"`dart pub publish`",
		"`fastlane deliver|supply`",
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief is missing %q\n%s", want, brief)
		}
	}
	if strings.Contains(brief, "- api: `dart test`") {
		t.Error("a package without a test/ folder gets no test command")
	}
	if strings.Contains(brief, "wallet/android") {
		t.Error("the Flutter app's generated android/ project gets no Gradle check")
	}
	if strings.Count(brief, "`mvn deploy`") != 1 {
		t.Error("Java and Kotlin share one publish guardrail")
	}
	if strings.Contains(brief, "gradle check") && !strings.Contains(brief, "./gradlew check") {
		t.Error("the checked-in wrapper is preferred")
	}
}
