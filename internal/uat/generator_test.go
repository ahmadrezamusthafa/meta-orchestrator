package uat

import (
	"strings"
	"testing"
)

func TestUATGenerator(t *testing.T) {
	gen := NewUATGenerator()

	spec := &UATSpecification{
		FeatureTitle:      "Passwordless Login",
		BusinessGoal:      "Enable users to authenticate via WebAuthn magic link",
		TargetEnvironment: "https://staging.app.internal",
		TestCredentials: map[string]string{
			"Standard User": "user@example.com",
			"Admin User":    "admin@example.com",
		},
		UserClickPaths: []string{
			"Navigate to /login",
			"Enter user@example.com into email input",
			"Click 'Send Magic Link' button",
			"Verify green confirmation banner appears",
		},
		EdgeCasesChecked: []string{
			"Malformed email address displays inline validation error",
			"Rate-limited after 5 requests in 1 minute",
		},
		VerificationRubrics: []string{
			"Magic link token expires in 15 minutes",
			"Session cookie is Set-Cookie HttpOnly and Secure",
		},
	}

	doc := gen.GenerateUATDocument(spec)

	if !strings.Contains(doc, "# User Acceptance Testing (UAT) Guide: Passwordless Login") {
		t.Errorf("missing title in UAT document")
	}
	if !strings.Contains(doc, "user@example.com") {
		t.Errorf("missing credentials in table")
	}
	if !strings.Contains(doc, "Click 'Send Magic Link' button") {
		t.Errorf("missing user click path step")
	}
	if !strings.Contains(doc, "Rate-limited after 5 requests") {
		t.Errorf("missing edge case")
	}
}
