# syntax=docker/dockerfile:1

# BuildKit sets BUILDPLATFORM itself; the default only keeps the legacy
# builder (docker without buildx) working, which leaves it empty.
ARG BUILDPLATFORM=linux/amd64

# Base images are pinned by digest so a build is reproducible; Dependabot
# (docker ecosystem) proposes the updates. The golang tag must match the go
# directive in go.mod (tests/test-pins.sh checks it).
FROM --platform=$BUILDPLATFORM node:24.22.0-alpine@sha256:44fce696f38ba8c987c4db89954051000bbd05ffd8d8659ba15ed4a8602e937f AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# The Go stage cross-compiles on the build platform; CGO is off, so no
# emulation is needed for the arm64 image.
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS go
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION#v}" -o /out/gotree ./cmd/gotree \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
LABEL org.opencontainers.image.licenses="AGPL-3.0-only AND CC-BY-4.0"
COPY --from=go /out/gotree /gotree
# The logo's CC BY 4.0 attribution travels with the image.
COPY LICENSE NOTICE /usr/share/doc/gotree/
# Created here so the volume starts out owned by the nonroot user.
COPY --from=go --chown=nonroot:nonroot /out/data /data
ENV GOTREE_DATA_DIR=/data GOTREE_LISTEN_ADDR=:8080
VOLUME /data
EXPOSE 8080
USER nonroot
# There is no shell or curl in the image: the binary checks itself, against
# the same GOTREE_LISTEN_ADDR the server listens on.
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 CMD ["/gotree", "-healthcheck"]
ENTRYPOINT ["/gotree"]
