package afclient

import (
	"context"
	"fmt"
	"net/url"
)

// Suggestions is the response of the suggested-versions endpoint: what the API proposes as the next version, the value for each increment level, the pre-release variants, and the current latest version. The action computes none of this — the API owns the SemVer logic.
//
// Only the fields the action exposes are modelled. Unknown fields are ignored by encoding/json, so additions upstream do not break this.
type Suggestions struct {
	SuggestedVersion string      `json:"suggested_version"`
	Increments       Increments  `json:"increments"`
	Latest           string      `json:"latest"`
	PreReleases      PreReleases `json:"pre_releases"`
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

// SuggestedVersions fetches the suggested next versions for one artefact. artefactID is the artefact's 24-hex id — artefacts are addressed by id here, not by name.
func (c *Client) SuggestedVersions(ctx context.Context, artefactID string) (*Suggestions, error) {
	var s Suggestions
	path := "/v1/artefacts/" + url.PathEscape(artefactID) + "/suggested-versions"
	if err := c.getJSON(ctx, path, nil, &s); err != nil {
		return nil, fmt.Errorf("get suggested versions for %s: %w", artefactID, err)
	}
	if s.SuggestedVersion == "" {
		return nil, fmt.Errorf("get suggested versions for %s: response contained no suggested_version", artefactID)
	}
	return &s, nil
}
