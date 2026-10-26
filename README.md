# Suggest Version

[![CI](https://github.com/artefactflow/suggest-version-action/actions/workflows/ci.yml/badge.svg)](https://github.com/artefactflow/suggest-version-action/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

## What is ArtefactFlow?

[ArtefactFlow](https://www.artefactflow.com) is a hosted platform for managing software artefacts and their Software Bills of Materials. It gives your team one source of truth for every application, library and image you ship: each artefact and its versions, the licenses and vulnerabilities they carry, the immutable CycloneDX SBOMs that describe them, and a record of which SBOM version is running in which environment. So when someone asks what is inside a release, whether it is vulnerable, or what exactly is in production right now, there is one answer rather than four.

Everything is available over a REST API, which is what makes integrations like this one possible. [Create an account](https://www.artefactflow.com) to get started, or read the [documentation](https://docs.artefactflow.com) for the full picture.

## What this action does

A GitHub Action that asks ArtefactFlow for an artefact's suggested next version and hands it to the rest of your workflow, to tag a commit, write a changelog entry, cut a release, or publish the version back to ArtefactFlow.

The action computes nothing itself. ArtefactFlow owns the SemVer logic and knows what the artefact's latest version is, so the answer is the same whichever pipeline asks. It is one of ArtefactFlow's GitHub integrations, and pairs with [publish-version-action](https://github.com/artefactflow/publish-version-action), which records the version once the build succeeds.

```yaml
- name: Get the next version
  id: suggest
  uses: artefactflow/suggest-version-action@v1
  with:
    client-id: ${{ secrets.ARTEFACTFLOW_CLIENT_ID }}
    client-secret: ${{ secrets.ARTEFACTFLOW_CLIENT_SECRET }}
    artefact-name: acme-payment-service

- name: Tag the release
  run: git tag "v${{ steps.suggest.outputs.version }}"
```

## Inputs

| Input | Required | Default | Environment variable | Description |
|---|---|---|---|---|
| `api-url` | no | `https://api.artefactflow.app` | `ARTEFACTFLOW_API_URL` | Base URL of the API. Override it only if you have been given a different endpoint |
| `client-id` | yes | n/a | `ARTEFACTFLOW_CLIENT_ID` | A service account's client id: its id, a UUID |
| `client-secret` | yes | n/a | `ARTEFACTFLOW_CLIENT_SECRET` | The service account's client secret. Pass it from a secret |
| `artefact-name` | yes | n/a | `ARTEFACTFLOW_ARTEFACT_NAME` | The artefact's name |
| `bump` | no | `suggested` | `ARTEFACTFLOW_BUMP` | Which suggestion becomes the `version` output: `suggested`, `major`, `minor`, `patch`, `alpha`, `beta`, or `rc` |
| `line` | no | newest line | `ARTEFACTFLOW_LINE` | The release line to suggest from, as its `major.minor` prefix (e.g. `1.4`). See [Release lines](#release-lines) |

### Naming the artefact

Artefacts are addressed by name, never by id. The match is exact and case-sensitive, and a name is unique within your organisation. Which organisation that is comes from your credentials, so nothing else is needed to disambiguate it.

The API addresses artefacts by id internally, so the action resolves the name first. That costs one extra call per run and needs the `artefact:read` scope alongside `version:read`.

### Configuring through the environment

Every input can come from an `ARTEFACTFLOW_`-prefixed environment variable instead of `with:`. A job-level `env:` block then configures every step at once:

```yaml
jobs:
  release:
    runs-on: ubuntu-latest
    env:
      ARTEFACTFLOW_CLIENT_ID: ${{ secrets.ARTEFACTFLOW_CLIENT_ID }}
      ARTEFACTFLOW_CLIENT_SECRET: ${{ secrets.ARTEFACTFLOW_CLIENT_SECRET }}
      ARTEFACTFLOW_ARTEFACT_NAME: acme-payment-service
    steps:
      - id: suggest
        uses: artefactflow/suggest-version-action@v1
        with:
          bump: minor   # a `with:` value always wins over the variable
```

`ARTEFACTFLOW_API_URL` is worth setting at the workflow or organisation level if you have been given a different API endpoint, so no individual step has to name it.

## Outputs

| Output | Description |
|---|---|
| `version` | The selected suggestion, per `bump`. Defaults to what the API suggests |
| `latest` | The artefact's highest existing version by SemVer precedence. `0.0.0` when nothing has been released; empty when a requested `line` has no versions yet |
| `major` | The next version with the major level incremented |
| `minor` | The next version with the minor level incremented |
| `patch` | The next version with the patch level incremented |
| `alpha` | The alpha pre-release variant of the suggested version |
| `beta` | The beta pre-release variant of the suggested version |
| `rc` | The release-candidate pre-release variant of the suggested version |
| `line` | The release line the suggestions belong to, e.g. `1.6`. Empty when the artefact has no versions yet |
| `lines` | Every release line in flight, newest first, comma-separated (e.g. `1.6,1.5,1.4`) |

Every variant is always emitted, whatever `bump` is set to: one call gives you all of them, so a workflow can read `patch` and `rc` from the same step.

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
| `line` | `1.2` |
| `lines` | `1.2,1.1` |

## Release lines

An artefact under parallel maintenance has more than one release line in flight: 1.4.x still being patched while 1.6.x is the newest. A single "next version" cannot serve both, so the action reports the lines it found in `lines` and tells you which one it answered from in `line`.

**By default the suggestions come from the newest line.** That is right for a workflow on your main branch and wrong on a maintenance branch: a job building 1.4.x would be handed `1.6.1` and tag the wrong release. Set `line` there:

```yaml
- uses: artefactflow/suggest-version-action@v1
  id: suggest
  with:
    client-id: ${{ secrets.ARTEFACTFLOW_CLIENT_ID }}
    client-secret: ${{ secrets.ARTEFACTFLOW_CLIENT_SECRET }}
    artefact-name: payment-service
    line: '1.4'          # patch the 1.4 line, whatever the newest line is
    bump: patch
```

Deriving it from the branch name works well when your branches are named for the line:

```yaml
- id: line
  run: echo "value=${GITHUB_REF_NAME#release/}" >> "$GITHUB_OUTPUT"   # release/1.4 -> 1.4
- uses: artefactflow/suggest-version-action@v1
  with:
    # ... credentials ...
    artefact-name: payment-service
    line: ${{ steps.line.outputs.value }}
```

A few things worth knowing:

- **A release line is not a Git branch.** Lines are derived from the artefact's existing version
  numbers, so they exist whether or not you keep a branch per line.
- **Suggestions never collide.** The API checks every value it returns against the versions that
  already exist, so a suggestion is always free to create. This matters most across lines: with
  1.5.0 already released, the next minor on the 1.4 line is `1.7.0`, not the taken `1.5.0`.
- **Asking for a line that does not exist yet** gives you its first version. `line: '2.0'` on an
  artefact with no 2.0.x answers `patch` = `2.0.0`, with `latest` empty because nothing has been
  released on it. `minor` and `major` still step off the line, to `2.1.0` and `3.0.0`.
- **Retired lines are not offered by default.** A line whose newest version is marked end-of-life
  is left out of `lines`, but naming it in `line` still answers about it, and its version numbers
  count as taken either way.

## Examples

### Pick the increment from the branch

Feature branches get a minor bump, everything else a patch:

```yaml
- id: suggest
  uses: artefactflow/suggest-version-action@v1
  with:
    client-id: ${{ secrets.ARTEFACTFLOW_CLIENT_ID }}
    client-secret: ${{ secrets.ARTEFACTFLOW_CLIENT_SECRET }}
    artefact-name: acme-payment-service
    bump: ${{ startsWith(github.head_ref, 'feature/') && 'minor' || 'patch' }}
```

### Release candidates on a branch, final versions on main

Both come from the same call, so the branch decides which output to read rather than which call to make:

```yaml
- id: suggest
  uses: artefactflow/suggest-version-action@v1
  with:
    client-id: ${{ secrets.ARTEFACTFLOW_CLIENT_ID }}
    client-secret: ${{ secrets.ARTEFACTFLOW_CLIENT_SECRET }}
    artefact-name: acme-payment-service

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
    client-id: ${{ secrets.ARTEFACTFLOW_CLIENT_ID }}
    client-secret: ${{ secrets.ARTEFACTFLOW_CLIENT_SECRET }}
    artefact-name: acme-payment-service

- env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    BODY: |
      Merging this releases **${{ steps.suggest.outputs.version }}** (currently ${{ steps.suggest.outputs.latest }}).
  run: gh pr comment "${{ github.event.number }}" --body "$BODY"
```

## Authentication

A service account is the only way the action authenticates. Create one in ArtefactFlow and give the action its **client id** and **client secret**; the action exchanges them for a short-lived token, uses it, and discards it. Nothing is cached between runs and no token is ever emitted as an output.

There is deliberately no way to hand the action an already-minted token. A token pasted into a workflow is usually a person's, which carries their full scope set, attributes every release to them rather than to a machine identity, and stops working the day they leave. Requiring a service account keeps the credential least-privileged and auditable by construction.

The client id is the service account's own id, a UUID, the same value shown as `id` when you create it. The secret starts with `afsa_` and is shown **once**, at creation and after a rotation. It cannot be retrieved later, and rotating invalidates the previous secret immediately, with no overlap, so update the repository secret in the same change.

Store both as encrypted secrets:

```yaml
with:
  client-id: ${{ secrets.ARTEFACTFLOW_CLIENT_ID }}
  client-secret: ${{ secrets.ARTEFACTFLOW_CLIENT_SECRET }}
```

The client id is not sensitive, so a repository or organisation **variable** is fine for it if you prefer (`${{ vars.ARTEFACTFLOW_CLIENT_ID }}`). The secret must be a secret.

### Scopes and team access

The service account needs both the `artefact:read` and `version:read` scopes: one to resolve the name, one to read the versions.

A higher action on the same resource covers a lower one: `version:write` and `version:delete` both satisfy `version:read`, and likewise for `artefact:`. So an account that already writes versions needs nothing added to read them. Its token will list only the highest action per resource, which is why a working account can show `version:write` and no `version:read` at all.

Scopes are fixed on the account when it is created and cannot be requested at token time; changing them means updating the account.

ArtefactFlow enforces a second axis alongside scopes: the account must belong to a team with read access to the artefact's group. A fresh service account with the right scopes but no team membership is still denied with a `403`. If you see one, check the team membership before the scopes.

A service account also carries its own expiry date, at most a year out. Once it passes, no new tokens can be minted and the action starts failing. Extend it before it lapses.

The client secret is never logged, and it is redacted from every error the action produces. The minted token is masked in the workflow log as soon as it is issued. Note that secrets are not available to workflows triggered by pull requests from forks, so gate any step that needs one.

## Permissions

The action itself only reads from the ArtefactFlow API and needs no GitHub token. Grant whatever the surrounding steps need. For the pull-request comment above, that is `pull-requests: write`.

## Requirements

This is a container action, so it runs on Linux runners only. Windows and macOS runners cannot run it.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `service-account credentials are required` | `client-id`/`client-secret` were not set. Check the secret names. An unset secret expands to an empty string silently |
| `input 'client-id' must be the service account's id, a UUID` | The account's name was passed instead of its id |
| `HTTP 401: the client credentials were rejected` | Wrong client id or secret. The API cannot distinguish the two. A rotation invalidates the old secret immediately |
| `HTTP 401: the service account ... is deactivated` | The account was deactivated, or its organisation is inactive |
| `HTTP 401: the service account has passed its expiry date` | Extend the account's expiry; it is capped at a year |
| `HTTP 403` | Missing `version:read` (or `artefact:read` when resolving a name), or the principal is not on a team with read access to the artefact's group |
| `resolve artefact "…": HTTP 404` | No artefact of that name in your organisation. The match is exact and case-sensitive |
| `the API returned no beta version for this artefact` | `bump` asked for a variant the API did not provide for this artefact |
| `input 'line' must be a release line's major.minor prefix` | `line` was given a full version (`1.4.2`), a branch name (`release/1.4`), or a bare major (`1`). It takes `major.minor` only |
| The version came back from the wrong line | `line` was not set, so the newest line answered. Set it on maintenance branches |

Errors quote the API's own message and its log id, which is what to include when asking for help with a `403` or a `500`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). To report a security issue, follow [SECURITY.md](SECURITY.md) instead of opening a public issue.

## License

[Apache-2.0](LICENSE)
