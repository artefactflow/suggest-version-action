// Package action adapts the ArtefactFlow suggested-versions response to the GitHub Actions runner: it reads the action inputs from the environment, picks the primary output according to the requested bump, and writes the outputs in the runner's file format.
package action

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Config is the validated set of action inputs.
type Config struct {
	APIURL     string
	Token      string
	ArtefactID string
	Bump       Bump
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

// artefactIDRE matches a 24-hex ObjectID. Validating locally turns the most common mistake — passing an artefact name — into a precise message instead of an opaque 404.
var artefactIDRE = regexp.MustCompile(`^[a-f0-9]{24}$`)

// LoadConfig reads and validates the action inputs. getenv is injected so tests do not have to mutate the process environment.
//
// The runner exposes each input as INPUT_<NAME> with the name upper-cased, so `api-url` arrives as INPUT_API-URL. Values are trimmed because YAML block scalars and copy-pasted secrets routinely carry a trailing newline.
func LoadConfig(getenv func(string) string) (Config, error) {
	input := func(name string) string {
		return strings.TrimSpace(getenv("INPUT_" + strings.ToUpper(name)))
	}

	cfg := Config{
		APIURL:     strings.TrimRight(input("api-url"), "/"),
		Token:      input("token"),
		ArtefactID: strings.ToLower(input("artefact-id")),
		Bump:       Bump(strings.ToLower(input("bump"))),
	}

	if cfg.APIURL == "" {
		return Config{}, fmt.Errorf("input `api-url` is required")
	}
	if !strings.HasPrefix(cfg.APIURL, "http://") && !strings.HasPrefix(cfg.APIURL, "https://") {
		return Config{}, fmt.Errorf("input `api-url` must be an absolute http(s) URL, got %q", cfg.APIURL)
	}
	if cfg.Token == "" {
		return Config{}, fmt.Errorf("input `token` is required. Pass it from a secret, for example ${{ secrets.ARTEFACTFLOW_TOKEN }}")
	}
	if cfg.ArtefactID == "" {
		return Config{}, fmt.Errorf("input `artefact-id` is required")
	}
	if !artefactIDRE.MatchString(cfg.ArtefactID) {
		return Config{}, fmt.Errorf("input `artefact-id` must be a 24-character hex artefact id, got %q — artefacts are addressed by id here, not by name", cfg.ArtefactID)
	}
	// An empty `bump` is the unset default rather than an error: action.yml declares a default, but a caller passing an empty expression bypasses it.
	if cfg.Bump == "" {
		cfg.Bump = BumpSuggested
	}
	if !validBumps[cfg.Bump] {
		return Config{}, fmt.Errorf("input `bump` must be one of %s, got %q", strings.Join(bumpNames(), ", "), cfg.Bump)
	}
	return cfg, nil
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
