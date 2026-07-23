// Package action adapts the ArtefactFlow suggested-versions response to the GitHub Actions runner: it reads the action inputs from the environment, picks the primary output according to the requested bump, and writes the outputs in the runner's file format.
package action

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// DefaultAPIURL is the hosted ArtefactFlow API, used when neither the `api-url` input nor ARTEFACTFLOW_API_URL is set. Self-hosted operators override it; nothing else in the action assumes a particular instance.
const DefaultAPIURL = "https://api.artefactflow.app"

// Config is the validated set of action inputs.
type Config struct {
	APIURL string
	// Service-account client credentials, exchanged for a short-lived token before anything else happens. They are the only way in: the action deliberately does not accept an already-minted token, so every run authenticates as a machine principal with its own scopes rather than borrowing a human's.
	ClientID     string
	ClientSecret string
	// ArtefactName is how the artefact is always addressed. The API needs an id, so the action resolves the name first — the id is an implementation detail that never appears in a workflow file.
	ArtefactName string
	Bump         Bump
}

// Bump selects which suggestion becomes the primary `version` output.
type Bump string

// The accepted values of the `bump` input. BumpSuggested is the default and passes through whatever the API proposes.
const (
	BumpSuggested Bump = "suggested"
	BumpMajor     Bump = "major"
	BumpMinor     Bump = "minor"
	BumpPatch     Bump = "patch"
	BumpAlpha     Bump = "alpha"
	BumpBeta      Bump = "beta"
	BumpRC        Bump = "rc"
)

var validBumps = map[Bump]bool{
	BumpSuggested: true,
	BumpMajor:     true,
	BumpMinor:     true,
	BumpPatch:     true,
	BumpAlpha:     true,
	BumpBeta:      true,
	BumpRC:        true,
}

// objectIDRE matches a 24-hex ObjectID, the form of a service account's client id.
var objectIDRE = regexp.MustCompile(`^[a-f0-9]{24}$`)

// artefactNameRE mirrors the character set the API accepts for an artefact name. Rejecting a bad name here means the resolve call is only made when it has a chance of succeeding.
var artefactNameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// EnvVar returns the environment variable that backs an input: `artefact-name` is backed by ARTEFACTFLOW_ARTEFACT_NAME. Keeping the mapping mechanical means a new input gets its variable for free.
func EnvVar(input string) string {
	return "ARTEFACTFLOW_" + strings.ToUpper(strings.ReplaceAll(input, "-", "_"))
}

// LoadConfig reads and validates the action inputs. getenv is injected so tests do not have to mutate the process environment.
//
// Each input can come from two places: the `with:` block, which the runner exposes as INPUT_<NAME> (so `api-url` arrives as INPUT_API-URL), or an ARTEFACTFLOW_-prefixed environment variable set at workflow, job, or step level. An explicit `with:` value wins, so a single job-level `env:` block can configure every step while one step still overrides, say, the bump.
//
// This is also why `api-url` and `bump` carry no `default:` in action.yml. The runner substitutes a declared default before the action ever runs, which would make the input always non-empty and the environment fallback dead code. Their defaults are applied here instead.
func LoadConfig(getenv func(string) string) (Config, error) {
	input := func(name string) string {
		if v := strings.TrimSpace(getenv("INPUT_" + strings.ToUpper(name))); v != "" {
			return v
		}
		return strings.TrimSpace(getenv(EnvVar(name)))
	}

	cfg := Config{
		APIURL:       strings.TrimRight(input("api-url"), "/"),
		ClientID:     strings.ToLower(input("client-id")),
		ClientSecret: input("client-secret"),
		ArtefactName: input("artefact-name"),
		Bump:         Bump(strings.ToLower(input("bump"))),
	}

	if cfg.APIURL == "" {
		cfg.APIURL = DefaultAPIURL
	}
	if !strings.HasPrefix(cfg.APIURL, "http://") && !strings.HasPrefix(cfg.APIURL, "https://") {
		return Config{}, fmt.Errorf("input `api-url` must be an absolute http(s) URL, got %q", cfg.APIURL)
	}
	if err := cfg.validateCredentials(); err != nil {
		return Config{}, err
	}

	if cfg.ArtefactName == "" {
		return Config{}, fmt.Errorf("input `artefact-name` is required (or the %s environment variable)", EnvVar("artefact-name"))
	}
	if err := validateArtefactName(cfg.ArtefactName); err != nil {
		return Config{}, err
	}

	// An empty `bump` is the unset default rather than an error.
	if cfg.Bump == "" {
		cfg.Bump = BumpSuggested
	}
	if !validBumps[cfg.Bump] {
		return Config{}, fmt.Errorf("input `bump` must be one of %s, got %q", strings.Join(bumpNames(), ", "), cfg.Bump)
	}
	return cfg, nil
}

// validateCredentials checks that a complete pair of service-account credentials was supplied.
//
// An already-minted bearer token is deliberately not accepted. Accepting one would make it easy to hand the action a human user's token, which carries that person's full scope set, attributes releases to a person rather than a machine identity, and stops working when they leave. Requiring a service account makes least privilege the only option rather than the diligent one.
//
// No message here ever quotes the secret — errors become annotations in a public log.
func (c Config) validateCredentials() error {
	switch {
	case c.ClientID == "" && c.ClientSecret == "":
		return fmt.Errorf("service-account credentials are required: set `client-id` and `client-secret` (or the %s / %s environment variables)", EnvVar("client-id"), EnvVar("client-secret"))
	case c.ClientID == "":
		return fmt.Errorf("`client-secret` was set without `client-id`")
	case c.ClientSecret == "":
		return fmt.Errorf("`client-id` was set without `client-secret`")
	// The client id is the service account's own 24-hex id. Checking it locally catches the common mistake of pasting the account name, which the API would answer with an indistinguishable "invalid credentials".
	case !objectIDRE.MatchString(c.ClientID):
		return fmt.Errorf("input `client-id` must be the service account's 24-character hex id, got %q", c.ClientID)
	}
	return nil
}

// validateArtefactName applies the limits the API documents for a name, so an unusable value fails before a request is made.
func validateArtefactName(name string) error {
	if len(name) < 2 || len(name) > 128 {
		return fmt.Errorf("input `artefact-name` must be between 2 and 128 characters, got %d", len(name))
	}
	if !artefactNameRE.MatchString(name) {
		return fmt.Errorf("input `artefact-name` may only contain letters, digits, underscores, and hyphens, got %q", name)
	}
	return nil
}

// bumpNames lists the accepted bump values in a stable order, for error messages.
func bumpNames() []string {
	names := make([]string, 0, len(validBumps))
	for b := range validBumps {
		names = append(names, string(b))
	}
	sort.Strings(names)
	return names
}
