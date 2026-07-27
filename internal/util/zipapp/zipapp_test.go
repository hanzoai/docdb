// Copyright 2021 Hanzo AI Inc.
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

package zipapp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zap-proto/zip"
)

// assertSame asserts that the app and the [http.ServeMux] answer req identically.
func assertSame(t *testing.T, app *zip.App, mux *http.ServeMux, req *http.Request) {
	t.Helper()

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res, err := app.Fiber().Test(req)
	require.NoError(t, err)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())

	assert.Equal(t, rec.Code, res.StatusCode)
	assert.Equal(t, rec.Body.String(), string(body))

	for _, h := range []string{"Content-Type", "X-Content-Type-Options"} {
		assert.Equal(t, rec.Header().Get(h), res.Header.Get(h), h)
	}
}

// TestNetHTTPParity asserts that a zipapp answers the requests no route handles
// exactly as the [http.ServeMux] it replaced did.
func TestNetHTTPParity(t *testing.T) {
	t.Parallel()

	app := New("test")
	app.Get("/known", func(c *zip.Ctx) error { return Text(c, http.StatusTeapot, "brewing") })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /known", func(rw http.ResponseWriter, _ *http.Request) {
		http.Error(rw, "brewing", http.StatusTeapot)
	})

	t.Run("Handled", func(t *testing.T) {
		assertSame(t, app, mux, httptest.NewRequest(http.MethodGet, "/known", nil))
	})

	t.Run("NotFound", func(t *testing.T) {
		assertSame(t, app, mux, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		assertSame(t, app, mux, httptest.NewRequest(http.MethodPost, "/known", nil))
	})
}
