# Contributing

Thanks for taking the time. Bug reports, documentation fixes, and pull requests are all welcome.

## Getting started

The action is a small Go program shipped as a container action. You need Go (the version in [go.mod](go.mod)) and, to build the image, Docker.

```sh
go build ./...
go test ./...
```

## Running it locally

The binary reads the same environment the runner gives it, so you can exercise it without a workflow. With `GITHUB_OUTPUT` unset it prints the outputs to stdout instead of writing them to a file. The `ARTEFACTFLOW_` variables are the convenient way in, because unlike the `INPUT_` ones they contain no hyphens, so a shell accepts them as an assignment prefix:

```sh
ARTEFACTFLOW_CLIENT_ID=... \
ARTEFACTFLOW_CLIENT_SECRET=... \
ARTEFACTFLOW_ARTEFACT_NAME=acme-payment-service \
ARTEFACTFLOW_BUMP=minor \
go run ./cmd/suggest-version
```

To exercise the `with:` path instead, set the `INPUT_` variables the runner would set. Those do contain hyphens, so they have to go through `env`:

```sh
env 'INPUT_ARTEFACT-NAME=acme-payment-service' 'INPUT_CLIENT-ID=...' 'INPUT_CLIENT-SECRET=...' go run ./cmd/suggest-version
```

Against the stub, with no ArtefactFlow instance and no credentials at all. [`testdata/stub-api.py`](testdata/stub-api.py) implements the three endpoints the action calls, and it is the same stub CI uses:

```sh
python3 testdata/stub-api.py 8099 &

ARTEFACTFLOW_API_URL=http://localhost:8099 \
ARTEFACTFLOW_CLIENT_ID=0192f5a0-7c3e-7b1a-9d2e-5f4c3b2a1d0e \
ARTEFACTFLOW_CLIENT_SECRET=afsa_stub-client-secret \
ARTEFACTFLOW_ARTEFACT_NAME=acme-payment-service \
go run ./cmd/suggest-version
```

The stub rejects any request that does not present the token it minted, so a change that breaks the authentication wiring fails there rather than passing quietly.

## Before opening a pull request

CI runs these, so running them first saves a round trip:

```sh
gofmt -l .        # must print nothing
go vet ./...
go test -race ./...
docker build .    # the action is a container action; the image has to build
```

CI additionally runs a smoke test that executes the action end to end against a stub API, which covers the wiring the unit tests cannot reach.

## Conventions

- Keep the response mapping covered by tests. The mapping tests decode the documented response shape rather than building structs by hand, so a renamed JSON tag is caught.
- The token must never reach a log line, an error message, or an output. There are tests asserting this; keep them passing.
- Failures surface as `::error::` annotations with a non-zero exit, so they land on the run summary rather than only in the step log.
- Prose in Markdown is written as one unwrapped line per paragraph. Do not hard-wrap at a column.
- `action.yml` stays at the repository root; that is a Marketplace requirement.
- Do not edit `CHANGELOG.md`.

## Releasing

Releases are cut by maintainers from the `Release` workflow. It re-runs the tests from the tag, builds and pushes the multi-architecture image to GHCR, creates the GitHub release, and repoints the moving major tag (`v1`) at it, skipping that last step for pre-releases.

`action.yml` references **one exact image version**, for example `docker://ghcr.io/artefactflow/suggest-version-action:1.2.3`, and that reference is part of the release commit. A floating `:1` would be less work, but it would mean someone who pins `uses: ...@v1.2.0` or a commit SHA silently gets a newer image on your next release. Pinning has to actually pin. The release workflow refuses to publish when the reference and the tag disagree.

So each release is:

1. Update `runs.image` in `action.yml` to the version being released, and commit.
2. Tag that commit (`git tag v1.2.3`) and push the tag.
3. Run the `Release` workflow against the tag, with **dry-run off**.

Between steps 2 and 3 the tag exists but its image does not, so the action is briefly unusable at that ref. Nothing consumes a tag before its release exists, so this is safe, but do not leave the gap open longer than necessary.

### The first release

The first release is the exception: no image exists yet, so `action.yml` stays on `image: 'Dockerfile'` and the workflow emits a notice rather than failing. That release builds on the runner for anyone who uses it. Immediately afterwards:

1. Confirm the package is **public**: GHCR packages are not reliably public just because the repository is, and a private image fails for every consumer with an unauthorized pull. Check under the organisation's Packages settings.
2. Switch `action.yml` to `docker://ghcr.io/artefactflow/suggest-version-action:<that version>` and cut the next release normally.

Listing on the Marketplace is a manual step on the first release only: open the release, choose **Edit**, and tick **Publish this Action to the GitHub Marketplace**.

### Why CI stages a copy of the action

Because `action.yml` points at a published image, `uses: ./` in the smoke job would pull that image and test the last release rather than the pull request. The job therefore copies the action into `ci-local-action/` with the image reference rewritten to `Dockerfile`, and runs that. If you add a file the container build needs, add it to that copy step too.
