package action

import (
	"strings"
	"testing"
)

// env builds a getenv function over a fixed map, so tests never touch the process environment.
func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func validEnv() map[string]string {
	return map[string]string{
		"INPUT_API-URL":     "https://api.example.com",
		"INPUT_TOKEN":       "secret-token",
		"INPUT_ARTEFACT-ID": "0123456789abcdef01234567",
		"INPUT_BUMP":        "minor",
	}
}

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig(env(validEnv()))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.APIURL != "https://api.example.com" {
		t.Errorf("APIURL = %q", cfg.APIURL)
	}
	if cfg.ArtefactID != "0123456789abcdef01234567" {
		t.Errorf("ArtefactID = %q", cfg.ArtefactID)
	}
	if cfg.Bump != BumpMinor {
		t.Errorf("Bump = %q", cfg.Bump)
	}
}

func TestLoadConfigNormalises(t *testing.T) {
	m := validEnv()
	// Trailing newlines on secrets and block scalars are routine; a trailing slash on the base URL is a common copy-paste.
	m["INPUT_API-URL"] = "  https://api.example.com/  \n"
	m["INPUT_TOKEN"] = "secret-token\n"
	m["INPUT_ARTEFACT-ID"] = "0123456789ABCDEF01234567"
	m["INPUT_BUMP"] = "RC"

	cfg, err := LoadConfig(env(m))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.APIURL != "https://api.example.com" {
		t.Errorf("APIURL not trimmed of space and trailing slash: %q", cfg.APIURL)
	}
	if cfg.Token != "secret-token" {
		t.Errorf("Token not trimmed: %q", cfg.Token)
	}
	if cfg.ArtefactID != "0123456789abcdef01234567" {
		t.Errorf("ArtefactID not lower-cased: %q", cfg.ArtefactID)
	}
	if cfg.Bump != BumpRC {
		t.Errorf("Bump not lower-cased: %q", cfg.Bump)
	}
}

func TestLoadConfigDefaultsBump(t *testing.T) {
	m := validEnv()
	m["INPUT_BUMP"] = ""
	cfg, err := LoadConfig(env(m))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Bump != BumpSuggested {
		t.Errorf("empty bump should default to %q, got %q", BumpSuggested, cfg.Bump)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantSub string
	}{
		{"missing api-url", func(m map[string]string) { m["INPUT_API-URL"] = "" }, "`api-url` is required"},
		{"relative api-url", func(m map[string]string) { m["INPUT_API-URL"] = "api.example.com" }, "absolute http(s) URL"},
		{"missing token", func(m map[string]string) { m["INPUT_TOKEN"] = "" }, "`token` is required"},
		{"missing artefact-id", func(m map[string]string) { m["INPUT_ARTEFACT-ID"] = "" }, "`artefact-id` is required"},
		{"artefact name instead of id", func(m map[string]string) { m["INPUT_ARTEFACT-ID"] = "my-service" }, "24-character hex"},
		{"artefact-id too short", func(m map[string]string) { m["INPUT_ARTEFACT-ID"] = "0123456789abcdef0123456" }, "24-character hex"},
		{"unknown bump", func(m map[string]string) { m["INPUT_BUMP"] = "hotfix" }, "`bump` must be one of"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := validEnv()
			tc.mutate(m)
			_, err := LoadConfig(env(m))
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not mention %q", err, tc.wantSub)
			}
		})
	}
}

// The token must never reach an error string — action errors are printed as annotations in a public log.
func TestLoadConfigErrorsDoNotLeakToken(t *testing.T) {
	m := validEnv()
	m["INPUT_ARTEFACT-ID"] = "not-an-id"
	_, err := LoadConfig(env(m))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), m["INPUT_TOKEN"]) {
		t.Fatalf("error leaked the token: %v", err)
	}
}
