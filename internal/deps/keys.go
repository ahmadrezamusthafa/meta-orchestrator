package deps

import (
	"regexp"
	"strings"
)

// keyDeps are the frameworks and tools worth showing first, per ecosystem. Go modules are matched
// without their /vN suffix; JVM names are matched as "group:artifact" prefixes.
var keyDeps = map[string][]string{
	"node": {"react", "react-dom", "vue", "next", "nuxt", "svelte", "@sveltejs/kit", "@angular/core", "vite",
		"typescript", "playwright", "@playwright/test", "jest", "vitest", "express", "nestjs", "@nestjs/core",
		"webpack", "react-native", "expo", "astro", "remix", "@remix-run/react"},
	"go": {"github.com/gin-gonic/gin", "github.com/labstack/echo", "github.com/gofiber/fiber", "google.golang.org/grpc",
		"gorm.io/gorm", "github.com/jackc/pgx", "github.com/go-chi/chi", "github.com/gorilla/mux", "github.com/stretchr/testify"},
	"flutter": {"flutter_bloc", "bloc", "riverpod", "flutter_riverpod", "hooks_riverpod", "dio", "provider", "go_router",
		"get", "freezed", "http"},
	"ruby":   {"rails", "sinatra", "rspec", "rspec-rails", "puma", "sidekiq"},
	"python": {"django", "flask", "fastapi", "pytest", "sqlalchemy", "pydantic", "celery"},
	"jvm": {"org.springframework.boot:", "org.jetbrains.kotlin:", "io.ktor:", "junit:junit", "org.junit.jupiter:",
		"io.quarkus:", "io.micronaut:", "androidx.compose"},
}

var goMajor = regexp.MustCompile(`/v[0-9]+$`)

func init() { keyDeps["dart"] = keyDeps["flutter"] }

func markKeys(e *Ecosystem) {
	keys := keyDeps[e.Ecosystem]
	for i := range e.Dependencies {
		e.Dependencies[i].Key = isKey(e.Ecosystem, e.Dependencies[i].Name, keys)
	}
}

func isKey(eco, name string, keys []string) bool {
	switch eco {
	case "go":
		name = goMajor.ReplaceAllString(name, "")
	case "python":
		name = pyNorm(name)
	}
	for _, k := range keys {
		if name == k || (eco == "jvm" && strings.HasPrefix(name, k)) {
			return true
		}
	}
	return false
}
