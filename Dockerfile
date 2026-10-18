# syntax=docker/dockerfile:1

# BuildKit sets BUILDPLATFORM itself; the default only keeps the legacy
# builder (docker without buildx) working, which leaves it empty.
ARG BUILDPLATFORM=linux/amd64

FROM --platform=$BUILDPLATFORM node:lts-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# The Go stage cross-compiles on the build platform; CGO is off, so no
# emulation is needed for the arm64 image.
FROM --platform=$BUILDPLATFORM golang:1-alpine AS go
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

FROM gcr.io/distroless/static-debian12:nonroot
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
