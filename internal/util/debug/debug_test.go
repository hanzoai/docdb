// Copyright 2021 DocDB Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package debug provides debug facilities.
package debug

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hanzoai/docdb/internal/util/must"
	"github.com/hanzoai/docdb/internal/util/testutil"
)

func assertProbe(t *testing.T, u string, expected int) {
	t.Helper()

	res, err := http.Get(u)
	require.NoError(t, err)
	assert.Equal(t, expected, res.StatusCode)
}

// get returns the status, headers and body of a GET request to u, without following redirects.
func get(t *testing.T, u string) (int, http.Header, string) {
	t.Helper()

	client := http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	res, err := client.Get(u)
	require.NoError(t, err)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())

	return res.StatusCode, res.Header, string(body)
}

func TestDebug(t *testing.T) {
	t.Parallel()

	var livez, readyz atomic.Bool

	reg := prometheus.NewRegistry()

	h := must.NotFail(Listen(&ListenOpts{
		TCPAddr: "127.0.0.1:0",
		L:       testutil.Logger(t),
		R:       reg,
		Livez:   func(context.Context) bool { return livez.Load() },
		Readyz:  func(context.Context) bool { return readyz.Load() },
	}))

	ctx, cancel := context.WithCancel(testutil.Ctx(t))
	done := make(chan struct{})

	go func() {
		h.Run(ctx)
		close(done)
	}()

	t.Run("Probes", func(t *testing.T) {
		live := "http://" + h.lis.Addr().String() + "/debug/livez"
		ready := "http://" + h.lis.Addr().String() + "/debug/readyz"

		assertProbe(t, live, http.StatusInternalServerError)
		assertProbe(t, ready, http.StatusInternalServerError)

		readyz.Store(true)

		assertProbe(t, live, http.StatusInternalServerError)
		assertProbe(t, ready, http.StatusInternalServerError)

		livez.Store(true)

		assertProbe(t, live, http.StatusOK)
		assertProbe(t, ready, http.StatusOK)
	})

	t.Run("Wire", func(t *testing.T) {
		testWire(t, h.lis.Addr().String(), reg, h.handlers, &livez, &readyz)
	})

	t.Run("Routes", func(t *testing.T) {
		root := "http://" + h.lis.Addr().String()

		t.Run("Table", func(t *testing.T) {
			paths := map[string]struct{}{}
			for _, route := range h.app.Fiber().GetRoutes(true) {
				paths[route.Path] = struct{}{}
			}

			assert.Equal(t, []string{
				"/*",
				"/debug",
				"/debug/archive",
				"/debug/archive.zip",
				"/debug/livez",
				"/debug/metrics",
				"/debug/readyz",
			}, slices.Sorted(maps.Keys(paths)))
		})

		t.Run("Index", func(t *testing.T) {
			code, header, body := get(t, root+"/debug")

			assert.Equal(t, http.StatusOK, code)
			assert.Equal(t, "text/html; charset=utf-8", header.Get("Content-Type"))

			for path := range h.handlers {
				assert.Contains(t, body, `<a href="`+path+`">`)
			}
		})

		t.Run("Metrics", func(t *testing.T) {
			code, _, body := get(t, root+"/debug/metrics")

			assert.Equal(t, http.StatusOK, code)
			assert.Contains(t, body, "# TYPE go_goroutines gauge")
		})

		t.Run("ArchiveZip", func(t *testing.T) {
			code, header, _ := get(t, root+"/debug/archive.zip")

			assert.Equal(t, http.StatusSeeOther, code)
			assert.Equal(t, "/debug/archive", header.Get("Location"))
		})

		// Handlers of other packages, served by [http.DefaultServeMux] behind the catch-all route.
		for _, path := range []string{"/debug/vars", "/debug/pprof/", "/debug/graphs/", "/debug/requests"} {
			t.Run(path, func(t *testing.T) {
				code, _, body := get(t, root+path)

				assert.Equal(t, http.StatusOK, code)
				assert.NotEmpty(t, body)
			})
		}

		// Anything else redirects to the index, as [http.DefaultServeMux]'s "/" handler always did.
		for _, path := range []string{"/", "/no-such-path"} {
			t.Run(path, func(t *testing.T) {
				code, header, _ := get(t, root+path)

				assert.Equal(t, http.StatusSeeOther, code)
				assert.Equal(t, "/debug", header.Get("Location"))
			})
		}
	})

	t.Run("Archive", func(t *testing.T) {
		u := "http://" + h.lis.Addr().String() + "/debug/archive"

		expectedFiles := map[string]struct{}{
			"version.json":    {},
			"buildinfo.json":  {},
			"metrics.txt":     {},
			"vars.json":       {},
			"profile.pprof":   {},
			"goroutine.pprof": {},
			"block.pprof":     {},
			"heap.pprof":      {},
			"trace.out":       {},
			"errors.txt":      {},
		}

		res, err := http.Get(u)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res.StatusCode)
		assert.Equal(t, "application/zip", res.Header.Get("Content-Type"))
		assert.Regexp(t, regexp.MustCompile(`attachment; filename=docdb-[\d-]+.zip`), res.Header.Get("Content-Disposition"))

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())

		zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		require.NoError(t, err)

		for _, file := range zipReader.File {
			name := file.FileHeader.Name
			assert.Contains(t, expectedFiles, file.FileHeader.Name)
			delete(expectedFiles, file.FileHeader.Name)

			t.Run(name, func(t *testing.T) {
				f, e := file.Open()
				require.NoError(t, e)

				defer func() {
					assert.NoError(t, f.Close())
				}()

				b, e := io.ReadAll(f)
				require.NoError(t, e)
				assert.NotEmpty(t, b)

				if name == "errors.txt" {
					t.Logf("\n%s", b)
				}
			})
		}

		assert.Empty(t, expectedFiles)
	})

	cancel()
	<-done // prevent panic on logging after test ends
}
