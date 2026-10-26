package afclient

import (
	"context"
	"fmt"
	"net/url"
)

// Suggestions is the response of the suggested-versions endpoint: what the API proposes as the next version, the value for each increment level, the pre-release variants, and the current latest version. The action computes none of this: the API owns the SemVer logic.
//
// Only the fields the action exposes are modelled. Unknown fields are ignored by encoding/json, so additions upstream do not break this.
type Suggestions struct {
	SuggestedVersion string       `json:"suggested_version"`
	Increments       Increments   `json:"increments"`
	Latest           string       `json:"latest"`
	PreReleases      PreReleases  `json:"pre_releases"`
	Lines            []LineResult `json:"lines"`
}

// LineResult is one release line in flight for the artefact: 1.4.x maintained alongside 1.6.x.
// The API derives these from the artefact's existing versions; the action only reports them.
//
// Note this is a release line, not a Git branch. An artefact's versions imply their own lines.
type LineResult struct {
	Line           string `json:"line"`
	Latest         string `json:"latest"`
	SuggestedPatch string `json:"suggested_patch"`
}

// Increments holds the next version at each SemVer increment level.
type Increments struct {
	Major string `json:"major"`
	Minor string `json:"minor"`
	Patch string `json:"patch"`
}

// PreReleases holds the pre-release variants of the suggested version.
type PreReleases struct {
	Alpha string `json:"alpha"`
	Beta  string `json:"beta"`
	RC    string `json:"rc"`
}

// artefactRef is the resolve endpoint's response: a minimal reference to one artefact.
type artefactRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ResolveArtefactID turns an artefact name into its id. The match is exact and case-sensitive, and a name is unique within an organisation. Which one that is comes from the token, so no further qualifier is needed.
//
// This is a separate call from the version lookup, and it needs its own scope: artefact:read here, version:read for the versions themselves.
func (c *Client) ResolveArtefactID(ctx context.Context, name string) (string, error) {
	q := url.Values{}
	q.Set("name", name)

	var ref artefactRef
	if err := c.getJSON(ctx, "/v1/artefacts/resolve", q, &ref); err != nil {
		return "", fmt.Errorf("resolve artefact %q: %w", name, err)
	}
	if ref.ID == "" {
		return "", fmt.Errorf("resolve artefact %q: response contained no id", name)
	}
	return ref.ID, nil
}

// SuggestedVersions fetches the suggested next versions for one artefact, addressed by its id.
//
// A non-empty line restricts the suggestions to that release line (`1.4`), which is what a
// workflow running on a maintenance branch needs: without it the API answers from the artefact's
// newest line, so a job building 1.4.x would be handed 1.6.1 and tag the wrong release.
func (c *Client) SuggestedVersions(ctx context.Context, artefactID, line string) (*Suggestions, error) {
	var s Suggestions
	path := "/v1/artefacts/" + url.PathEscape(artefactID) + "/suggested-versions"

	var q url.Values
	if line != "" {
		q = url.Values{}
		q.Set("line", line)
	}

	if err := c.getJSON(ctx, path, q, &s); err != nil {
		return nil, fmt.Errorf("get suggested versions for %s: %w", artefactID, err)
	}
	if s.SuggestedVersion == "" {
		return nil, fmt.Errorf("get suggested versions for %s: response contained no suggested_version", artefactID)
	}
	return &s, nil
}
