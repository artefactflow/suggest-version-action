# Suggest Version

[![CI](https://github.com/artefactflow/suggest-version-action/actions/workflows/ci.yml/badge.svg)](https://github.com/artefactflow/suggest-version-action/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

A GitHub Action that asks ArtefactFlow for an artefact's suggested next version and hands it to the rest of your workflow — to tag a commit, write a changelog entry, cut a release, or publish the version back to ArtefactFlow.

The action computes nothing itself. ArtefactFlow owns the SemVer logic and knows what the artefact's latest version is, so the answer is the same whichever pipeline asks.

```yaml
- name: Get the next version
  id: suggest
  uses: artefactflow/suggest-version-action@v1
  with:
    api-url: https://api.artefactflow.example
    token: ${{ secrets.ARTEFACTFLOW_TOKEN }}
    artefact-id: 0123456789abcdef01234567

- name: Tag the release
  run: git tag "v${{ steps.suggest.outputs.version }}"
```

## Inputs

| Input | Required | Default | Description |
|---|---|---|---|
| `api-url` | yes | — | Base URL of your ArtefactFlow API |
| `token` | yes | — | Bearer token with the `artefact:read` scope. Pass it from a secret — never inline it |
| `artefact-id` | yes | — | The artefact's 24-character hex id. Artefacts are addressed by id, not by name |
| `bump` | no | `suggested` | Which suggestion becomes the `version` output: `suggested`, `major`, `minor`, `patch`, `alpha`, `beta`, or `rc` |

## Outputs

| Output | Description |
|---|---|
| `version` | The selected suggestion, per `bump`. Defaults to what the API suggests |
| `latest` | The artefact's current latest version. Empty when nothing has been released yet |
| `major` | The next version with the major level incremented |
| `minor` | The next version with the minor level incremented |
| `patch` | The next version with the patch level incremented |
| `alpha` | The alpha pre-release variant of the suggested version |
| `beta` | The beta pre-release variant of the suggested version |
| `rc` | The release-candidate pre-release variant of the suggested version |

Every variant is always emitted, whatever `bump` is set to — one call gives you all of them, so a workflow can read `patch` and `rc` from the same step.

For an artefact whose latest version is `1.2.3`, the outputs look like this:

| Output | Value |
|---|---|
| `version` | `1.2.4` |
| `latest` | `1.2.3` |
| `major` | `2.0.0` |
| `minor` | `1.3.0` |
| `patch` | `1.2.4` |
| `alpha` | `1.2.4-alpha.1` |
| `beta` | `1.2.4-beta.1` |
| `rc` | `1.2.4-rc.1` |

## Examples

### Pick the increment from the branch

Feature branches get a minor bump, everything else a patch:

```yaml
- id: suggest
  uses: artefactflow/suggest-version-action@v1
  with:
    api-url: ${{ vars.ARTEFACTFLOW_API_URL }}
    token: ${{ secrets.ARTEFACTFLOW_TOKEN }}
    artefact-id: ${{ vars.ARTEFACT_ID }}
    bump: ${{ startsWith(github.head_ref, 'feature/') && 'minor' || 'patch' }}
```

### Release candidates on a branch, final versions on main

Both come from the same call, so the branch decides which output to read rather than which call to make:

```yaml
- id: suggest
  uses: artefactflow/suggest-version-action@v1
  with:
    api-url: ${{ vars.ARTEFACTFLOW_API_URL }}
    token: ${{ secrets.ARTEFACTFLOW_TOKEN }}
    artefact-id: ${{ vars.ARTEFACT_ID }}

- name: Build
  env:
    VERSION: ${{ github.ref == 'refs/heads/main' && steps.suggest.outputs.version || steps.suggest.outputs.rc }}
  run: docker build -t "myapp:$VERSION" .
```

### Comment the next version on a pull request

```yaml
- id: suggest
  uses: artefactflow/suggest-version-action@v1
  with:
    api-url: ${{ vars.ARTEFACTFLOW_API_URL }}
    token: ${{ secrets.ARTEFACTFLOW_TOKEN }}
    artefact-id: ${{ vars.ARTEFACT_ID }}

- env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    BODY: |
      Merging this releases **${{ steps.suggest.outputs.version }}** (currently ${{ steps.suggest.outputs.latest }}).
  run: gh pr comment "${{ github.event.number }}" --body "$BODY"
```

## Authentication

The token needs the `artefact:read` scope, and ArtefactFlow enforces a second axis alongside it: the token's principal must belong to a team with read access to the artefact's group. A fresh service account with the right scope but no team membership is still denied with a `403` — if you see one, check the team membership before the scope.

Pass the token from a secret. It is never logged, and it is redacted from every error the action produces. Note that secrets are not available to workflows triggered by pull requests from forks, so gate any step that needs one.

## Permissions

The action itself only reads from the ArtefactFlow API and needs no GitHub token. Grant whatever the surrounding steps need — for the pull-request comment above, that is `pull-requests: write`.

## Requirements

This is a container action, so it runs on Linux runners only. Windows and macOS runners cannot run it.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `input 'artefact-id' must be a 24-character hex artefact id` | An artefact name was passed instead of an id |
| `HTTP 401: the token was rejected` | The token is invalid, expired, or belongs to another instance |
| `HTTP 403` | Missing the `artefact:read` scope, or the principal is not on a team with read access to the artefact's group |
| `HTTP 404: no such artefact` | The id is well-formed but no artefact has it on this instance |
| `the API returned no beta version for this artefact` | `bump` asked for a variant the API did not provide for this artefact |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). To report a security issue, follow [SECURITY.md](SECURITY.md) instead of opening a public issue.

## License

[Apache-2.0](LICENSE)
