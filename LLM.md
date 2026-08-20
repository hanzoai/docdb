# Hanzo Documentdb

## Overview
Go module: github.com/hanzoai/docdb

## Tech Stack
- **Language**: Go
- **HTTP**: every listener (Data API, MCP, debug) routes through
  `internal/util/zipapp` on `github.com/zap-proto/zip` -- ZAP is the wire,
  HTTP is a view of the same routes. Do not reach for `net/http.ServeMux`.

## Build & Run
```bash
go build ./...
go test ./...
```

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

## Structure
```
documentdb/
  CHANGELOG.md
  CODE_OF_CONDUCT.md
  CONTRIBUTING.md
  LICENSE
  README.md
  SECURITY.md
  Taskfile.yml
  cmd/
  docdb/
  docker-compose.yml
  go.mod
  go.sum
  integration/
  internal/
  tools/
```

## Key Files
- `README.md` -- Project documentation
- `go.mod` -- Go module definition
