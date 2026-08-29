# DocDB

## Overview
Go module: github.com/hanzoai/docdb

DocDB answers the MongoDB wire protocol and stores nothing itself: it
translates each command into SQL and runs it against PostgreSQL with the
DocumentDB extension. That is the whole of `main` — there is no second
backend, no `internal/backends/`, no `--handler` flag, and no SQLite.

## Two lineages wear this name, and only one of them is `main`
`main` is the v2 line: PostgreSQL + DocumentDB extension, `DOCDB_*` environment
variables. The managed product a customer provisions is NOT built from this
tree — `hanzoai/cloud` starts `ghcr.io/hanzoai/docdb-sqlite:1.24.0`, which is
the v1 line on SQLite with `FERRETDB_*` environment variables, built from the
`platform/mirror-v1-sqlite` branch. Read a bug report for "docdb" against the
image it names, not against this tree, until the two are reconciled.

The `v1.24.x` tags belong to that v1 line but were cut on `main`, on commits
NEWER than `v2.8.2`. `git describe` therefore answers `v1.24.5` here, which is
why `build/version/generate.go` excludes them: the string it writes is what
`buildInfo` and `serverStatus` hand a connected client, so a plain describe
makes every binary introduce itself as the other lineage.

## Tech Stack
- **Language**: Go
- **HTTP**: every listener (Data API, MCP, debug) routes through
  `internal/util/zipapp` on `github.com/zap-proto/zip` -- ZAP is the wire,
  HTTP is a view of the same routes. Do not reach for `net/http.ServeMux`.

## Build & Run
```bash
go build ./...
go generate ./build/version          # version.txt, or every build says "unknown"
go test -short -tags=docdb_dev ./... # the CI gate; -short skips what needs a database
```

`-tags=docdb_dev` is the repo's `BUILD_TAGS`; without it the dev-only packages
are excluded and their tests silently do not run.

To exercise the wire protocol, give it a database and drop `-short`:

```bash
docker compose up -d postgres
go test -tags=docdb_dev -run 'TestDocDB|TestCRUD' ./docdb/
```

`docdb/docdb_test.go` starts an embedded DocDB and drives it with the real
MongoDB driver, so it fails if anything from the socket down is wrong. The
`username` role those tests connect as is created by `bin/envtool setup`,
which needs the whole `task env-up` environment, not just Postgres.

### Go builder images
Every Go stage in `build/docdb/*.Dockerfile` pins `golang:<version>-bookworm`
to the same version the root `go.mod` `go` directive asks for, and sets
`ENV GOTOOLCHAIN=auto`.

The official `golang` images ship `GOTOOLCHAIN=local`. With `local`, a base
image older than the `go` directive is a hard build failure
(`go.mod requires go >= X (running go Y; GOTOOLCHAIN=local)`) rather than a
toolchain download. Dependabot bumps these `FROM` lines on its own cadence
(`.github/dependabot.yml`, docker ecosystem, `/build/docdb`) while `go.mod` is
bumped separately, so the two drift apart routinely. `auto` makes the drift
self-healing: Go fetches the required toolchain, checksum-verified against
sum.golang.org. The `-prepare` stage still has `GOPROXY` set, and its module
cache is shared with the `-build` stage via `--mount=type=cache,target=/cache`,
so a downloaded toolchain reaches the build stage even though it sets
`GOPROXY=off`.

## Upstream libraries keep their upstream path
This repo is a fork of FerretDB, but three of its dependencies are generic
upstream libraries we do not modify: `github.com/FerretDB/gh` (rate-limit-aware
GitHub client), `github.com/FerretDB/xfail` (expected-failure test helper), and
`github.com/FerretDB/wire` (MongoDB wire protocol). They are required and
imported under their upstream path, in every module, always.

Only the fork itself is renamed to `github.com/hanzoai/docdb`. A rename applied
to a dependency's path is not a fork -- it changes the label while `go.sum`
still pins the upstream bytes, and Go then has nothing to resolve: any repo at
the new path would have to ship a `go.mod` declaring the old one. Renaming a
dependency means owning a real fork, with its own tag and its own hashes.

These three sit outside `GOPRIVATE`, so they resolve through
proxy.golang.org and verify against sum.golang.org.

The rule is not about Go modules; it is about anything unmodified we consume by
name, and it broke twice more the same way. `build/deps/postgres-documentdb.Dockerfile`
was renamed to `ghcr.io/hanzoai/postgres-documentdb-dev:17-docdb`, which nobody
publishes, so `docker compose up postgres` could not pull and took the whole
development environment with it. `.gitmodules` was renamed to
`https://github.com/hanzoai/docdb.git` branch `docdb` — the fork itself, which
does not contain the commit the submodule pins, so `git submodule update` could
not check it out and `go generate ./internal/mongoerrors` lost the
`error_mappings.csv` and `documentdb_codes.txt` it reads. Both now name
upstream, where the bytes actually are.

Pushing code to git.hanzo.ai does not make it resolvable. Go fetches a
`github.com/hanzoai/...` module from github.com -- `GOPRIVATE` only skips the
proxy, and the one `insteadOf` rule for the forge rewrites `https://git.hanzo.ai`,
never github.com. So the forge is where the source is kept, not where an import
path points: a requirement is fixable only by naming a path that is actually
served, which for an unmodified upstream library means the upstream's own.

## The ZAP listener is off unless someone asks for it
`internal/zap` reaches the same DocumentDB pool the MongoDB port does and
authenticates nothing, over a plaintext transport, so the port is the whole of
the access control. `--listen-zap-addr` therefore defaults to `-`: no address,
no listener, and `internal/zap` never constructed. `--listen-addr` is
`127.0.0.1:27017` with `--auth` on, and the two postures now agree.

`--listen-zap-addr` is the only input. There is no `os.Getenv` left in the
package — `ZAP_PORT` and `ZAP_DISABLED` used to answer alongside the flag, and
`Config.Enabled` decided a second time what `setup.go` had already decided.
Kong's `DefaultEnvars("DOCDB")` still gives the flag `DOCDB_LISTEN_ZAP_ADDR`,
which is the flag, spelled for an environment.

A host in the address is REFUSED rather than dropped: `luxfi/zap` listens on
`fmt.Sprintf(":%d", port)` and `NodeConfig` has no bind field, so
`--listen-zap-addr=127.0.0.1:9654` used to read as bound and answer as open.
Write `:9654` and mean it.

Honoring a host needs a bind address in `luxfi/zap`'s `NodeConfig`, and
encrypting the transport needs its `TLS` field filled. Both are upstream work
in that package. Until then the node also advertises itself over mDNS as
`_hanzo-documentdb._tcp`.

`cmd/docdb.TestZAPListenerIsOffByDefault` and
`internal/zap.TestListenerServesOnlyWhenStarted` hold the two halves.

## Structure
```
docdb/
  CHANGELOG.md
  CODE_OF_CONDUCT.md
  CONTRIBUTING.md
  LICENSE
  NOTICE
  README.md
  SECURITY.md
  Taskfile.yml
  compose.yml
  build/          # Dockerfiles, nfpm packaging, systemd unit, version generator
  cmd/            # docdb (the server), envtool (dev/CI helper)
  docdb/          # the embeddable package, and the wire tests
  go.mod
  go.sum
  integration/    # its own module; needs a live PostgreSQL and MongoDB
  internal/
  tools/          # its own module
```

## Key Files
- `README.md` -- Project documentation
- `go.mod` -- Go module definition
- `hanzo.yml` -- the CI gate; the forge reads `.hanzo/workflows/cicd.yml`
