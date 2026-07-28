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
