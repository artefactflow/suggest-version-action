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
		"INPUT_API-URL":       "https://api.example.com",
		"INPUT_CLIENT-ID":     "0192f5a0-7c3e-7b1a-9d2e-5f4c3b2a1d0e",
		"INPUT_CLIENT-SECRET": "afsa_secret",
		"INPUT_ARTEFACT-NAME": "acme-payment-service",
		"INPUT_BUMP":          "minor",
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
	if cfg.ArtefactName != "acme-payment-service" {
		t.Errorf("ArtefactName = %q", cfg.ArtefactName)
	}
	if cfg.Bump != BumpMinor {
		t.Errorf("Bump = %q", cfg.Bump)
	}
}

func TestEnvVar(t *testing.T) {
	for input, want := range map[string]string{
		"api-url":       "ARTEFACTFLOW_API_URL",
		"client-id":     "ARTEFACTFLOW_CLIENT_ID",
		"client-secret": "ARTEFACTFLOW_CLIENT_SECRET",
		"artefact-name": "ARTEFACTFLOW_ARTEFACT_NAME",
		"bump":          "ARTEFACTFLOW_BUMP",
	} {
		if got := EnvVar(input); got != want {
			t.Errorf("EnvVar(%q) = %q, want %q", input, got, want)
		}
	}
}

// Every input can come from an ARTEFACTFLOW_ variable instead of a `with:` block.
func TestLoadConfigFromEnvironment(t *testing.T) {
	cfg, err := LoadConfig(env(map[string]string{
		"ARTEFACTFLOW_API_URL":       "https://self-hosted.example.com",
		"ARTEFACTFLOW_CLIENT_ID":     "0192f5a0-7c3e-7b1a-9d2e-5f4c3b2a1d0e",
		"ARTEFACTFLOW_CLIENT_SECRET": "afsa_secret",
		"ARTEFACTFLOW_ARTEFACT_NAME": "acme-payment-service",
		"ARTEFACTFLOW_BUMP":          "rc",
	}))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.APIURL != "https://self-hosted.example.com" {
		t.Errorf("APIURL = %q", cfg.APIURL)
	}
	if cfg.ClientSecret != "afsa_secret" {
		t.Errorf("ClientSecret = %q", cfg.ClientSecret)
	}
	if cfg.ArtefactName != "acme-payment-service" {
		t.Errorf("ArtefactName = %q", cfg.ArtefactName)
	}
	if cfg.Bump != BumpRC {
		t.Errorf("Bump = %q", cfg.Bump)
	}
}

// A `with:` value beats the environment, so one job-level env block can set the defaults and a single step can still override.
func TestInputWinsOverEnvironment(t *testing.T) {
	m := validEnv()
	m["ARTEFACTFLOW_API_URL"] = "https://ignored.example.com"
	m["ARTEFACTFLOW_BUMP"] = "major"

	cfg, err := LoadConfig(env(m))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.APIURL != "https://api.example.com" {
		t.Errorf("APIURL = %q, want the `with:` value", cfg.APIURL)
	}
	if cfg.Bump != BumpMinor {
		t.Errorf("Bump = %q, want the `with:` value", cfg.Bump)
	}
}

// The hosted API is the default, so a workflow against it only has to supply a token and an artefact.
func TestLoadConfigDefaultsAPIURL(t *testing.T) {
	cfg, err := LoadConfig(env(map[string]string{
		"INPUT_CLIENT-ID":     "0192f5a0-7c3e-7b1a-9d2e-5f4c3b2a1d0e",
		"INPUT_CLIENT-SECRET": "afsa_secret",
		"INPUT_ARTEFACT-NAME": "acme-payment-service",
	}))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.APIURL != DefaultAPIURL {
		t.Errorf("APIURL = %q, want %q", cfg.APIURL, DefaultAPIURL)
	}
	if DefaultAPIURL != "https://api.artefactflow.app" {
		t.Errorf("DefaultAPIURL = %q", DefaultAPIURL)
	}
	if cfg.Bump != BumpSuggested {
		t.Errorf("Bump = %q, want %q", cfg.Bump, BumpSuggested)
	}
}

func TestLoadConfigNormalises(t *testing.T) {
	m := validEnv()
	// Trailing newlines on secrets and block scalars are routine; a trailing slash on the base URL is a common copy-paste.
	m["INPUT_API-URL"] = "  https://api.example.com/  \n"
	m["INPUT_CLIENT-SECRET"] = "afsa_secret\n"
	m["INPUT_BUMP"] = "RC"

	cfg, err := LoadConfig(env(m))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.APIURL != "https://api.example.com" {
		t.Errorf("APIURL not trimmed of space and trailing slash: %q", cfg.APIURL)
	}
	if cfg.ClientSecret != "afsa_secret" {
		t.Errorf("ClientSecret not trimmed: %q", cfg.ClientSecret)
	}
	if cfg.Bump != BumpRC {
		t.Errorf("Bump not lower-cased: %q", cfg.Bump)
	}
}

// An artefact name is matched exactly and case-sensitively by the API, so its case must survive.
func TestLoadConfigPreservesNameCase(t *testing.T) {
	m := validEnv()
	m["INPUT_ARTEFACT-NAME"] = "Acme-Payment-Service"

	cfg, err := LoadConfig(env(m))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ArtefactName != "Acme-Payment-Service" {
		t.Errorf("ArtefactName = %q, want the original case", cfg.ArtefactName)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantSub string
	}{
		{"relative api-url", func(m map[string]string) { m["INPUT_API-URL"] = "api.example.com" }, "absolute http(s) URL"},
		{"missing credentials", func(m map[string]string) {
			m["INPUT_CLIENT-ID"] = ""
			m["INPUT_CLIENT-SECRET"] = ""
		}, "service-account credentials are required"},
		{"no artefact", func(m map[string]string) { m["INPUT_ARTEFACT-NAME"] = "" }, "`artefact-name` is required"},
		{"name too short", func(m map[string]string) { m["INPUT_ARTEFACT-NAME"] = "a" }, "between 2 and 128"},
		{"name with illegal characters", func(m map[string]string) { m["INPUT_ARTEFACT-NAME"] = "acme/payment service" }, "letters, digits, underscores"},
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

func TestLoadConfigClientCredentials(t *testing.T) {
	cfg, err := LoadConfig(env(map[string]string{
		"INPUT_CLIENT-ID":     "0192F5A0-7C3E-7B1A-9D2E-5F4C3B2A1D0E",
		"INPUT_CLIENT-SECRET": "afsa_secret",
		"INPUT_ARTEFACT-NAME": "acme-payment-service",
	}))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ClientID != "0192f5a0-7c3e-7b1a-9d2e-5f4c3b2a1d0e" {
		t.Errorf("ClientID = %q, want it lower-cased", cfg.ClientID)
	}
	if cfg.ClientSecret != "afsa_secret" {
		t.Errorf("ClientSecret = %q", cfg.ClientSecret)
	}
}

// An already-minted token is deliberately not an accepted credential: it would let a human user's token stand in for a service account. A `token` input must stay unrecognised rather than quietly working.
func TestLoadConfigRejectsATokenInput(t *testing.T) {
	for _, key := range []string{"INPUT_TOKEN", "ARTEFACTFLOW_TOKEN"} {
		m := map[string]string{
			"INPUT_ARTEFACT-NAME": "acme-payment-service",
			key:                   "a-borrowed-jwt",
		}
		_, err := LoadConfig(env(m))
		if err == nil {
			t.Fatalf("%s: expected an error, because a token must not satisfy the credential requirement", key)
		}
		if !strings.Contains(err.Error(), "service-account credentials are required") {
			t.Errorf("%s: error %q should point at the client credentials", key, err)
		}
	}
}

func TestLoadConfigCredentialErrors(t *testing.T) {
	base := map[string]string{"INPUT_ARTEFACT-NAME": "acme-payment-service"}
	tests := []struct {
		name    string
		creds   map[string]string
		wantSub string
	}{
		{"none at all", nil, "service-account credentials are required"},
		{"secret without id", map[string]string{"INPUT_CLIENT-SECRET": "afsa_secret"}, "without `client-id`"},
		{"id without secret", map[string]string{"INPUT_CLIENT-ID": "0192f5a0-7c3e-7b1a-9d2e-5f4c3b2a1d0e"}, "without `client-secret`"},
		{"account name as client id", map[string]string{
			"INPUT_CLIENT-ID":     "ci-service-account",
			"INPUT_CLIENT-SECRET": "afsa_secret",
		}, "a UUID"},
		{"24-hex client id", map[string]string{
			"INPUT_CLIENT-ID":     "507f191e810c19729de860ed",
			"INPUT_CLIENT-SECRET": "afsa_secret",
		}, "a UUID"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := map[string]string{}
			for k, v := range base {
				m[k] = v
			}
			for k, v := range tc.creds {
				m[k] = v
			}
			_, err := LoadConfig(env(m))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not mention %q", err, tc.wantSub)
			}
			if strings.Contains(err.Error(), "afsa_secret") {
				t.Errorf("error leaked the client secret: %v", err)
			}
		})
	}
}

// The client secret must never reach an error string: action errors are printed as annotations in a public log. Every validation failure is checked, not just one, because any of them could be the message that carries it.
func TestLoadConfigErrorsDoNotLeakTheSecret(t *testing.T) {
	const secret = "afsa_a-distinctive-secret-value"
	mutations := map[string]func(map[string]string){
		"bad name":      func(m map[string]string) { m["INPUT_ARTEFACT-NAME"] = "not a valid name" },
		"missing name":  func(m map[string]string) { m["INPUT_ARTEFACT-NAME"] = "" },
		"bad client id": func(m map[string]string) { m["INPUT_CLIENT-ID"] = "not-an-id" },
		"bad api url":   func(m map[string]string) { m["INPUT_API-URL"] = "api.example.com" },
		"bad bump":      func(m map[string]string) { m["INPUT_BUMP"] = "hotfix" },
		"id without pair": func(m map[string]string) {
			m["INPUT_CLIENT-ID"] = ""
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			m := validEnv()
			m["INPUT_CLIENT-SECRET"] = secret
			mutate(m)

			_, err := LoadConfig(env(m))
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error leaked the client secret: %v", err)
			}
		})
	}
}

// TestLoadConfig_Line covers the `line` input: accepted forms, the unset default, and the
// rejection that turns a typo into a named input error rather than an API 400 about a query
// parameter the workflow author never wrote.
func TestLoadConfig_Line(t *testing.T) {
	base := map[string]string{
		"INPUT_CLIENT-ID":     "0192f5a0-8d4f-7c2b-8e3f-6a5d4c3b2e1f",
		"INPUT_CLIENT-SECRET": "s3cret",
		"INPUT_ARTEFACT-NAME": "payment-service",
	}
	load := func(line string) (Config, error) {
		env := map[string]string{}
		for k, v := range base {
			env[k] = v
		}
		if line != "" {
			env["INPUT_LINE"] = line
		}
		return LoadConfig(func(k string) string { return env[k] })
	}

	for _, ok := range []string{"1.4", "0.1", "10.0", "1.10"} {
		cfg, err := load(ok)
		if err != nil {
			t.Errorf("line %q must be accepted: %v", ok, err)
			continue
		}
		if cfg.Line != ok {
			t.Errorf("line = %q, want %q", cfg.Line, ok)
		}
	}

	for _, bad := range []string{"1", "1.4.2", "v1.4", "1.x", "01.4", "1.04", "main", "release/1.4"} {
		if _, err := load(bad); err == nil {
			t.Errorf("line %q must be rejected", bad)
		}
	}

	cfg, err := load("")
	if err != nil {
		t.Fatalf("an unset line is the default, not an error: %v", err)
	}
	if cfg.Line != "" {
		t.Errorf("unset line = %q, want empty", cfg.Line)
	}
}
