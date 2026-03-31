# syntax=docker/dockerfile:1

# evaluation image (formerly "all-in-one") with production release (`docdb-eval` image)

# While we already know commit and version from commit.txt and version.txt inside image,
# it is not possible to use them in LABELs for the final image.
# We need to pass them as build arguments.
# Defining ARGs there makes them global.
ARG LABEL_VERSION
ARG LABEL_COMMIT


# prepare stage

# TODO https://github.com/hanzoai/docdb/issues/5449
FROM --platform=$BUILDPLATFORM golang:1.26.1-bookworm AS eval-prepare

# use a single directory for all Go caches to simplify RUN --mount commands below
ENV GOPATH=/cache/gopath
ENV GOCACHE=/cache/gocache
ENV GOMODCACHE=/cache/gomodcache

# remove ",direct"
ENV GOPROXY=https://proxy.golang.org

COPY go.mod go.sum /src/

WORKDIR /src

RUN --mount=type=cache,target=/cache <<EOF
set -ex

go mod download
go mod verify
EOF


# build stage

# TODO https://github.com/hanzoai/docdb/issues/5449
FROM golang:1.26.1-bookworm AS eval-build

ARG TARGETARCH

ARG LABEL_VERSION
ARG LABEL_COMMIT
RUN test -n "$LABEL_VERSION"
RUN test -n "$LABEL_COMMIT"

# use the same directories for Go caches as above
ENV GOPATH=/cache/gopath
ENV GOCACHE=/cache/gocache
ENV GOMODCACHE=/cache/gomodcache

# modules are already downloaded
ENV GOPROXY=off

# see .dockerignore
WORKDIR /src
COPY . .

# to add a dependency
COPY --from=eval-prepare /src/go.mod /src/go.sum /src/

RUN --mount=type=cache,target=/cache <<EOF
set -ex

git status

# Do not raise without providing separate builds with those values
# because higher versions are problematic for some virtualization platforms and older hardware.
export GOAMD64=v1
export GOARM64=v8.0

export CGO_ENABLED=0

go env

# Trim paths mostly to check that building with `-trimpath` is supported.

# check if stdlib was cached
go install -v -trimpath std

go build -v -trimpath -o=bin/docdb ./cmd/docdb

go version -m bin/docdb
bin/docdb --version
EOF


# final stage

# Use production image and full tag close to the release.
# FROM ghcr.io/hanzoai/postgres-documentdb:17-0.108.0-docdb-2.8.0 AS eval

# Use moving development image during development.
FROM ghcr.io/hanzoai/postgres-documentdb-dev:17-docdb AS eval

RUN --mount=type=cache,sharing=locked,target=/var/cache/apt <<EOF
apt install -y curl supervisor
curl -L https://pgp.mongodb.com/server-7.0.asc | apt-key add -
echo "deb [ arch=amd64,arm64 ] https://repo.mongodb.org/apt/debian bookworm/mongodb-org/7.0 main" | tee /etc/apt/sources.list.d/mongodb-org-7.0.list
apt update
apt install -y mongodb-mongosh
EOF

COPY --from=eval-build /src/bin/docdb /usr/local/bin/docdb

COPY --from=eval-build /src/build/docdb/evaluation/supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY --from=eval-build /src/build/docdb/evaluation/entrypoint.sh /usr/local/bin/entrypoint.sh

ENTRYPOINT ["entrypoint.sh"]

HEALTHCHECK --interval=1m --timeout=5s --retries=1 --start-period=30s --start-interval=5s \
  CMD ["/usr/local/bin/docdb", "ping"]

VOLUME /state
EXPOSE 27017 27018 8088

# don't forget to update documentation if you change defaults
ENV DOCDB_LISTEN_ADDR=:27017
# ENV DOCDB_LISTEN_TLS=:27018
ENV DOCDB_DEBUG_ADDR=:8088
ENV DOCDB_STATE_DIR=/state

ARG LABEL_VERSION
ARG LABEL_COMMIT

# TODO https://github.com/hanzoai/docdb/issues/2212
LABEL org.opencontainers.image.description="A truly Open Source MongoDB alternative (evaluation image)"
LABEL org.opencontainers.image.revision="${LABEL_COMMIT}"
LABEL org.opencontainers.image.source="https://github.com/hanzoai/docdb"
LABEL org.opencontainers.image.title="DocDB (evaluation image)"
LABEL org.opencontainers.image.url="https://www.docdb.hanzo.ai/"
LABEL org.opencontainers.image.vendor="DocDB Inc."
LABEL org.opencontainers.image.version="${LABEL_VERSION}"
