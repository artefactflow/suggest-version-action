# Suggest Version

[![CI](https://github.com/artefactflow/suggest-version-action/actions/workflows/ci.yml/badge.svg)](https://github.com/artefactflow/suggest-version-action/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

## What is ArtefactFlow?

[ArtefactFlow](https://www.artefactflow.com) is a hosted platform for managing software artefacts and their Software Bills of Materials. It gives your team one source of truth for every application, library and image you ship: each artefact and its versions, the licenses and vulnerabilities they carry, the immutable CycloneDX SBOMs that describe them, and a record of which SBOM version is running in which environment. So when someone asks what is inside a release, whether it is vulnerable, or what exactly is in production right now, there is one answer rather than four.

Everything is available over a REST API, which is what makes integrations like this one possible. [Create an account](https://www.artefactflow.com) to get started, or read the [documentation](https://docs.artefactflow.com) for the full picture.

## What this action does

A GitHub Action that asks ArtefactFlow for an artefact's suggested next version and hands it to the rest of your workflow — to tag a commit, write a changelog entry, cut a release, or publish the version back to ArtefactFlow.

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
| `api-url` | no | `https://api.artefactflow.app` | `ARTEFACTFLOW_API_URL` | Base URL of the API. Override it if you self-host |
| `client-id` | yes | — | `ARTEFACTFLOW_CLIENT_ID` | A service account's client id — its 24-character hex id |
| `client-secret` | yes | — | `ARTEFACTFLOW_CLIENT_SECRET` | The service account's client secret. Pass it from a secret |
| `artefact-name` | yes | — | `ARTEFACTFLOW_ARTEFACT_NAME` | The artefact's name |
| `bump` | no | `suggested` | `ARTEFACTFLOW_BUMP` | Which suggestion becomes the `version` output: `suggested`, `major`, `minor`, `patch`, `alpha`, `beta`, or `rc` |

### Naming the artefact

Artefacts are addressed by name, never by id. The match is exact and case-sensitive, and a name is unique within your organisation — which organisation that is comes from your credentials, so nothing else is needed to disambiguate it.

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

`ARTEFACTFLOW_API_URL` is worth setting at the workflow or organisation level when you self-host, so no individual step has to name your instance.

## Outputs

| Output | Description |
|---|---|
| `version` | The selected suggestion, per `bump`. Defaults to what the API suggests |
| `latest` | The artefact's current latest version, or `0.0.0` when nothing has been released yet |
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

The client id is the service account's own 24-character hex id — the same value shown as `id` when you create it. The secret starts with `afsa_` and is shown **once**, at creation and after a rotation. It cannot be retrieved later, and rotating invalidates the previous secret immediately, with no overlap — so update the repository secret in the same change.

Store both as encrypted secrets:

```yaml
with:
  client-id: ${{ secrets.ARTEFACTFLOW_CLIENT_ID }}
  client-secret: ${{ secrets.ARTEFACTFLOW_CLIENT_SECRET }}
```

The client id is not sensitive, so a repository or organisation **variable** is fine for it if you prefer (`${{ vars.ARTEFACTFLOW_CLIENT_ID }}`). The secret must be a secret.

### Scopes and team access

The service account needs both the `artefact:read` and `version:read` scopes — one to resolve the name, one to read the versions.

A higher action on the same resource covers a lower one: `version:write` and `version:delete` both satisfy `version:read`, and likewise for `artefact:`. So an account that already writes versions needs nothing added to read them. Its token will list only the highest action per resource, which is why a working account can show `version:write` and no `version:read` at all.

Scopes are fixed on the account when it is created and cannot be requested at token time; changing them means updating the account.

ArtefactFlow enforces a second axis alongside scopes: the account must belong to a team with read access to the artefact's group. A fresh service account with the right scopes but no team membership is still denied with a `403` — if you see one, check the team membership before the scopes.

A service account also carries its own expiry date, at most a year out. Once it passes, no new tokens can be minted and the action starts failing — extend it before it lapses.

The client secret is never logged, and it is redacted from every error the action produces. The minted token is masked in the workflow log as soon as it is issued. Note that secrets are not available to workflows triggered by pull requests from forks, so gate any step that needs one.

## Permissions

The action itself only reads from the ArtefactFlow API and needs no GitHub token. Grant whatever the surrounding steps need — for the pull-request comment above, that is `pull-requests: write`.

## Requirements

This is a container action, so it runs on Linux runners only. Windows and macOS runners cannot run it.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `service-account credentials are required` | `client-id`/`client-secret` were not set. Check the secret names — an unset secret expands to an empty string silently |
| `input 'client-id' must be the service account's 24-character hex id` | The account's name was passed instead of its id |
| `HTTP 401: the client credentials were rejected` | Wrong client id or secret. The API cannot distinguish the two. A rotation invalidates the old secret immediately |
| `HTTP 401: the service account ... is deactivated` | The account was deactivated, or its organisation is inactive |
| `HTTP 401: the service account has passed its expiry date` | Extend the account's expiry; it is capped at a year |
| `HTTP 403` | Missing `version:read` (or `artefact:read` when resolving a name), or the principal is not on a team with read access to the artefact's group |
| `resolve artefact "…": HTTP 404` | No artefact of that name in your organisation. The match is exact and case-sensitive |
| `the API returned no beta version for this artefact` | `bump` asked for a variant the API did not provide for this artefact |

Errors quote the API's own message and its log id, which is what to include when asking for help with a `403` or a `500`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). To report a security issue, follow [SECURITY.md](SECURITY.md) instead of opening a public issue.

## License

[Apache-2.0](LICENSE)
