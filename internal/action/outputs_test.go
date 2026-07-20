package action

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/artefactflow/suggest-version-action/internal/afclient"
)

// sampleBody is the documented response shape. Decoding it rather than building the struct by hand is the point of the mapping test: it catches a renamed or dropped JSON tag.
const sampleBody = `{
  "suggested_version": "1.2.4",
  "increments": { "major": "2.0.0", "minor": "1.3.0", "patch": "1.2.4" },
  "latest": "1.2.3",
  "pre_releases": { "alpha": "1.2.4-alpha.1", "beta": "1.2.4-beta.1", "rc": "1.2.4-rc.1" }
}`

func sample(t *testing.T) *afclient.Suggestions {
	t.Helper()
	var s afclient.Suggestions
	if err := json.Unmarshal([]byte(sampleBody), &s); err != nil {
		t.Fatalf("decode sample: %v", err)
	}
	return &s
}

func TestDecodeResponseShape(t *testing.T) {
	s := sample(t)
	cases := map[string]string{
		"suggested_version":  s.SuggestedVersion,
		"latest":             s.Latest,
		"increments.major":   s.Increments.Major,
		"increments.minor":   s.Increments.Minor,
		"increments.patch":   s.Increments.Patch,
		"pre_releases.alpha": s.PreReleases.Alpha,
		"pre_releases.beta":  s.PreReleases.Beta,
		"pre_releases.rc":    s.PreReleases.RC,
	}
	want := map[string]string{
		"suggested_version":  "1.2.4",
		"latest":             "1.2.3",
		"increments.major":   "2.0.0",
		"increments.minor":   "1.3.0",
		"increments.patch":   "1.2.4",
		"pre_releases.alpha": "1.2.4-alpha.1",
		"pre_releases.beta":  "1.2.4-beta.1",
		"pre_releases.rc":    "1.2.4-rc.1",
	}
	for field, got := range cases {
		if got != want[field] {
			t.Errorf("%s = %q, want %q", field, got, want[field])
		}
	}
}

func TestSelect(t *testing.T) {
	s := sample(t)
	want := map[Bump]string{
		BumpSuggested: "1.2.4",
		BumpMajor:     "2.0.0",
		BumpMinor:     "1.3.0",
		BumpPatch:     "1.2.4",
		BumpAlpha:     "1.2.4-alpha.1",
		BumpBeta:      "1.2.4-beta.1",
		BumpRC:        "1.2.4-rc.1",
	}
	for bump, expected := range want {
		got, err := Select(s, bump)
		if err != nil {
			t.Errorf("Select(%q): %v", bump, err)
			continue
		}
		if got != expected {
			t.Errorf("Select(%q) = %q, want %q", bump, got, expected)
		}
	}
}

func TestSelectMissingVariant(t *testing.T) {
	// An artefact with no pre-release variants still answers the call; asking for one must fail loudly rather than yield "".
	s := &afclient.Suggestions{SuggestedVersion: "1.0.0"}
	if _, err := Select(s, BumpBeta); err == nil {
		t.Fatal("expected an error for a missing beta version")
	}
}

func TestSelectUnknownBump(t *testing.T) {
	if _, err := Select(sample(t), Bump("hotfix")); err == nil {
		t.Fatal("expected an error for an unknown bump")
	}
}

func TestOutputs(t *testing.T) {
	outputs, err := Outputs(sample(t), BumpMajor)
	if err != nil {
		t.Fatalf("Outputs: %v", err)
	}
	got := map[string]string{}
	for _, o := range outputs {
		got[o.Name] = o.Value
	}
	want := map[string]string{
		"version": "2.0.0", // resolved per bump
		"latest":  "1.2.3",
		"major":   "2.0.0",
		"minor":   "1.3.0",
		"patch":   "1.2.4",
		"alpha":   "1.2.4-alpha.1",
		"beta":    "1.2.4-beta.1",
		"rc":      "1.2.4-rc.1",
	}
	if len(got) != len(want) {
		t.Errorf("got %d outputs, want %d: %v", len(got), len(want), got)
	}
	for name, expected := range want {
		if got[name] != expected {
			t.Errorf("output %s = %q, want %q", name, got[name], expected)
		}
	}
}

func TestWriteOutputs(t *testing.T) {
	var b strings.Builder
	outputs := []Output{{"version", "1.2.4"}, {"latest", "1.2.3"}}
	if err := WriteOutputs(&b, outputs); err != nil {
		t.Fatalf("WriteOutputs: %v", err)
	}
	want := "version=1.2.4\nlatest=1.2.3\n"
	if b.String() != want {
		t.Errorf("got %q, want %q", b.String(), want)
	}
}

// A line break in a value would let an upstream response forge extra outputs in the $GITHUB_OUTPUT file. Nothing is written when one is present.
func TestWriteOutputsRejectsLineBreaks(t *testing.T) {
	var b strings.Builder
	outputs := []Output{{"version", "1.2.4\nadmin=true"}}
	if err := WriteOutputs(&b, outputs); err == nil {
		t.Fatal("expected an error for a value containing a line break")
	}
	if b.String() != "" {
		t.Errorf("nothing should have been written, got %q", b.String())
	}
}
