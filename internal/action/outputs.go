package action

import (
	"fmt"
	"io"
	"strings"

	"github.com/artefactflow/suggest-version-action/internal/afclient"
)

// Output is one action output. A slice keeps the write order stable, which makes the step log and the tests readable.
type Output struct {
	Name  string
	Value string
}

// Select returns the suggestion the caller asked for. It is the only place the `bump` input is interpreted.
func Select(s *afclient.Suggestions, bump Bump) (string, error) {
	var v string
	switch bump {
	case BumpSuggested:
		v = s.SuggestedVersion
	case BumpMajor:
		v = s.Increments.Major
	case BumpMinor:
		v = s.Increments.Minor
	case BumpPatch:
		v = s.Increments.Patch
	case BumpAlpha:
		v = s.PreReleases.Alpha
	case BumpBeta:
		v = s.PreReleases.Beta
	case BumpRC:
		v = s.PreReleases.RC
	default:
		return "", fmt.Errorf("unknown bump %q", bump)
	}
	if v == "" {
		// The API answered, but not with the variant that was asked for. Failing here is better than handing a downstream tag step an empty string.
		return "", fmt.Errorf("the API returned no %s version for this artefact", bump)
	}
	return v, nil
}

// Outputs maps a response to the full set of action outputs, with `version` resolved per bump. Every variant is emitted regardless of the bump, so a workflow can read, say, both `patch` and `rc` from one call.
func Outputs(s *afclient.Suggestions, bump Bump) ([]Output, error) {
	version, err := Select(s, bump)
	if err != nil {
		return nil, err
	}
	return []Output{
		{"version", version},
		{"latest", s.Latest},
		{"major", s.Increments.Major},
		{"minor", s.Increments.Minor},
		{"patch", s.Increments.Patch},
		{"alpha", s.PreReleases.Alpha},
		{"beta", s.PreReleases.Beta},
		{"rc", s.PreReleases.RC},
		{"line", lineOf(s)},
		{"lines", linesOf(s)},
	}, nil
}

// lineOf reports which release line the suggestions belong to.
//
// It is read from the response rather than echoed from the `line` input, so it is answered
// whether or not the input was set: a workflow that did not ask for a line still learns which
// one it was given, which is the difference between "1.6.1 is the next version" and "1.6.1 is the
// next version *of the 1.6 line*, and 1.4.x is also in flight".
func lineOf(s *afclient.Suggestions) string {
	if len(s.Lines) == 0 {
		return ""
	}
	return s.Lines[0].Line
}

// linesOf renders every release line in flight as a comma-separated list, newest first, so a
// workflow can branch on whether an artefact is under parallel maintenance without parsing JSON.
func linesOf(s *afclient.Suggestions) string {
	if len(s.Lines) == 0 {
		return ""
	}
	names := make([]string, 0, len(s.Lines))
	for _, l := range s.Lines {
		names = append(names, l.Line)
	}
	return strings.Join(names, ",")
}

// WriteOutputs renders outputs in the runner's $GITHUB_OUTPUT file format.
//
// Version strings are single-line by definition, so the plain `name=value` form is used, and a value that is not single-line is rejected rather than written, because a newline in that file is how an untrusted value would forge additional outputs.
func WriteOutputs(w io.Writer, outputs []Output) error {
	var b strings.Builder
	for _, o := range outputs {
		if strings.ContainsAny(o.Value, "\r\n") {
			return fmt.Errorf("refusing to write output %q: the API returned a value containing a line break", o.Name)
		}
		fmt.Fprintf(&b, "%s=%s\n", o.Name, o.Value)
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write outputs: %w", err)
	}
	return nil
}
