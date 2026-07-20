# Contributing

Thanks for taking the time. Bug reports, documentation fixes, and pull requests are all welcome.

## Getting started

The action is a small Go program shipped as a container action. You need Go (the version in [go.mod](go.mod)) and, to build the image, Docker.

```sh
go build ./...
go test ./...
```

## Running it locally

The binary reads the same environment variables the runner sets, so you can exercise it without a workflow. With `GITHUB_OUTPUT` unset it prints the outputs to stdout instead of writing them to a file. The input names contain hyphens, which a shell will not accept as an assignment prefix, so set them through `env`:

```sh
env 'INPUT_API-URL=https://api.artefactflow.example' \
    'INPUT_TOKEN=...' \
    'INPUT_ARTEFACT-ID=0123456789abcdef01234567' \
    'INPUT_BUMP=minor' \
    go run ./cmd/suggest-version
```

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
- Prose in Markdown is written as one unwrapped line per paragraph — do not hard-wrap at a column.
- `action.yml` stays at the repository root; that is a Marketplace requirement.
- Do not edit `CHANGELOG.md`.

## Releasing

Releases are cut by maintainers from the `Release` workflow. It builds and pushes the container image, creates the GitHub release, and repoints the moving major tag (`v1`) at it. Listing on the Marketplace is a manual step on the first release only.
