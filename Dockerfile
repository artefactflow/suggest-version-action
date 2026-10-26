# Build a static suggest-version binary and ship it on a minimal runtime.
#
# The runtime image carries CA certificates: the action makes an outbound HTTPS call to the ArtefactFlow API, so a bare scratch image would fail TLS verification. distroless static provides them at a fraction of the size of a full base image.
#
# There is deliberately no USER instruction. A container action runs as root on the runner, and the runner's workspace and file-command files are owned accordingly. An unprivileged user cannot append to $GITHUB_OUTPUT.
FROM golang:1.27 AS build
WORKDIR /src

# Module metadata first for layer caching. The module has no external dependencies, so this is effectively a no-op today, but it keeps the cache warm if any are added later.
COPY go.mod ./
RUN go mod download

COPY . .
ARG VERSION=dev
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/suggest-version ./cmd/suggest-version

FROM gcr.io/distroless/static-debian13 AS runtime
COPY --from=build /out/suggest-version /usr/local/bin/suggest-version
ENTRYPOINT ["/usr/local/bin/suggest-version"]
