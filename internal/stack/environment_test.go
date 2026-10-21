package stack

import (
	"strings"
	"testing"
)

func TestBriefEnvironments(t *testing.T) {
	api := Detect(writeRepo(t, map[string]string{
		"go.mod":                       "module px\nrequire github.com/jackc/pgx/v5 v5.0.0",
		"docs/ARCHITECTURE.md":         "# Architecture",
		"docs/adr/architecture-001.md": "deeper docs are not listed",
		"docker-compose.yml":           "services:\n  postgres:\n    image: postgres:16\n",
		".sdlc/constraints.md":         "- Production DB_DSN points at the prod DB firewall, never at PostgreSQL directly.\n",
	}), Deep)
	brief := Brief([]Repo{{Name: "px-api", Profile: api}})
	for _, want := range []string{
		"## Environments",
		"never change a production value to match local",
		"Read before changing configuration, networking or data access: px-api/docs/ARCHITECTURE.md\n",
		"From px-api/.sdlc/constraints.md:\n- Production DB_DSN points at the prod DB firewall",
		"- px-api: `docker compose -f docker-compose.yml up -d --wait` — only before e2e tests or evidence",
		"never hard-code a database host",
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief is missing %q\n%s", want, brief)
		}
	}

	plain := Brief([]Repo{{Name: "svc", Profile: Detect(writeRepo(t, map[string]string{"go.mod": "module svc"}), Deep)}})
	if strings.Contains(plain, "## Environments") {
		t.Errorf("no architecture docs or constraints means no Environments section:\n%s", plain)
	}
}

func TestConstraintsAreBounded(t *testing.T) {
	p := Detect(writeRepo(t, map[string]string{"go.mod": "module m", ".sdlc/constraints.md": strings.Repeat("x", 2*maxConstraints)}), Deep)
	if len(p.constraints) != maxConstraints {
		t.Errorf("constraints = %d bytes, want the first %d", len(p.constraints), maxConstraints)
	}
}
