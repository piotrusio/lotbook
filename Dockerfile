# syntax=docker/dockerfile:1
# check=skip=InvalidDefaultArgInFrom

# The image of Lotbook: one static binary, started in a role by its first
# argument (ADR-0001) — `lotbook ingest`, `lotbook worker`, `lotbook migrate`.
#
# Build it with `make image`. GO_VERSION has no default: it is the `go` line
# of go.mod, the one place the toolchain is pinned, and the Makefile passes it;
# a build without it fails, which is why the check above is skipped.

ARG GO_VERSION

# Compiled on the builder's own platform and cross-compiled to the target's:
# a pure-Go binary needs no emulation to build for another architecture.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -tags timetzdata -ldflags='-s -w' -o /out/lotbook ./cmd/lotbook

# Nothing but the binary and the CA roots it needs to speak TLS: no shell, no
# package manager, nothing else to patch.
FROM scratch
ARG REVISION=unknown
LABEL org.opencontainers.image.title="lotbook" \
      org.opencontainers.image.description="Lot genealogy engine" \
      org.opencontainers.image.source="https://github.com/piotrusio/lotbook" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.revision="$REVISION"
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/lotbook /lotbook
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/lotbook"]
