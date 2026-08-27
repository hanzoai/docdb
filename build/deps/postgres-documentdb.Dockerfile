# The DocumentDB extension image is upstream's, unmodified, and so keeps
# upstream's path — the same rule LLM.md states for gh, xfail and wire. The
# rename to ghcr.io/hanzoai/postgres-documentdb-dev pointed at an image nobody
# publishes, so `docker compose up postgres` failed to pull and took the whole
# development environment, and every integration test with it, down.

# Use development image and full tag close to the release.
# FROM ghcr.io/ferretdb/postgres-documentdb-dev:17-0.108.0-ferretdb-2.8.0

# Use moving development image during development.
FROM ghcr.io/ferretdb/postgres-documentdb-dev:17-ferretdb
