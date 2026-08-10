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

package dataapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hanzoai/docdb/internal/dataapi/api"
	"github.com/hanzoai/docdb/internal/util/testutil"
)

// actions are the paths of the Data API actions, all of them POST-only.
var actions = []string{
	"/action/aggregate",
	"/action/deleteMany",
	"/action/deleteOne",
	"/action/find",
	"/action/findOne",
	"/action/insertMany",
	"/action/insertOne",
	"/action/updateMany",
	"/action/updateOne",
}

// do sends req to a Data API router and returns the response status, headers and body.
//
// The router has no handler behind it, so requests must not reach one:
// routing, authentication and request decoding are all that can be exercised.
func do(t *testing.T, auth bool, req *http.Request) (int, http.Header, string) {
	t.Helper()

	app := newApp(t.Context(), &ListenOpts{L: testutil.Logger(t), Auth: auth})

	res, err := app.Fiber().Test(req)
	require.NoError(t, err)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())

	return res.StatusCode, res.Header, string(body)
}

// action returns a request for the given Data API action path.
func action(method, path, contentType string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(`{"database":"db","collection":"c"}`))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return req
}

// TestSpecRoutes asserts the router serves exactly the operations the OpenAPI
// specification describes, plus the specification itself.
//
// It stands in for the compile-time [api.ServerInterface] check the generated
// net/http handler used to provide, and covers more: methods and paths, not
// just the presence of a handler method per operation.
func TestSpecRoutes(t *testing.T) {
	t.Parallel()

	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}

	require.NoError(t, json.Unmarshal(api.Spec, &spec))
	require.NotEmpty(t, spec.Paths)

	// Fiber answers HEAD on every route registered with Get, so the spec's one
	// GET arrives as a pair. Nothing registers HEAD — the router adds it.
	expected := []string{"GET /openapi.json", "HEAD /openapi.json"}

	for path, operations := range spec.Paths {
		for method := range operations {
			m := strings.ToUpper(method)
			expected = append(expected, m+" "+path)

			if m == http.MethodGet {
				expected = append(expected, http.MethodHead+" "+path)
			}
		}
	}

	app := newApp(t.Context(), &ListenOpts{L: testutil.Logger(t)})

	var actual []string

	for _, route := range app.Fiber().GetRoutes(true) {
		actual = append(actual, route.Method+" "+route.Path)
	}

	slices.Sort(expected)
	slices.Sort(actual)

	assert.Equal(t, expected, actual)
}

func TestRoutes(t *testing.T) {
	t.Parallel()

	t.Run("OpenAPISpec", func(t *testing.T) {
		t.Parallel()

		code, header, body := do(t, false, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "application/json", header.Get("Content-Type"))
		assert.Equal(t, strconv.Itoa(len(api.Spec)), header.Get("Content-Length"))
		assert.Equal(t, string(api.Spec), body)
	})

	t.Run("Actions", func(t *testing.T) {
		t.Parallel()

		for _, path := range actions {
			t.Run(path, func(t *testing.T) {
				t.Parallel()

				// The request reaches the handler, which fails to decode a body that is not JSON.
				code, header, body := do(t, false, action(http.MethodPost, path, "text/plain"))

				assert.Equal(t, http.StatusInternalServerError, code)
				assert.Equal(t, "text/plain; charset=utf-8", header.Get("Content-Type"))
				assert.Contains(t, body, "Content-Type must be set to application/json")
			})
		}
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		t.Parallel()

		for _, path := range append([]string{"/openapi.json"}, actions...) {
			t.Run(path, func(t *testing.T) {
				t.Parallel()

				// the method each route does not declare
				method := http.MethodGet
				if path == "/openapi.json" {
					method = http.MethodPost
				}

				code, _, body := do(t, false, action(method, path, "application/json"))

				assert.Equal(t, http.StatusMethodNotAllowed, code)
				assert.Equal(t, "Method Not Allowed\n", body)
			})
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		code, header, body := do(t, false, action(http.MethodPost, "/action/nope", "application/json"))

		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, "text/plain; charset=utf-8", header.Get("Content-Type"))
		assert.Equal(t, "404 page not found\n", body)
	})

	t.Run("Auth", func(t *testing.T) {
		t.Parallel()

		// Authentication guards the specification too, as it guarded the whole mux before.
		for _, path := range append([]string{"/openapi.json"}, actions...) {
			t.Run(path, func(t *testing.T) {
				t.Parallel()

				method := http.MethodPost
				if path == "/openapi.json" {
					method = http.MethodGet
				}

				t.Run("NoCredentials", func(t *testing.T) {
					t.Parallel()

					code, header, body := do(t, true, action(method, path, "application/json"))

					assert.Equal(t, http.StatusBadRequest, code)
					assert.Equal(t, "application/json", header.Get("Content-Type"))
					assert.JSONEq(t, `{
						"error": "no authentication methods were specified",
						"error_code": "InvalidParameter"
					}`, body)
				})

				t.Run("EmptyPassword", func(t *testing.T) {
					t.Parallel()

					req := action(method, path, "application/json")
					req.SetBasicAuth("username", "")

					code, header, body := do(t, true, req)

					assert.Equal(t, http.StatusBadRequest, code)
					assert.Equal(t, "application/json", header.Get("Content-Type"))
					assert.JSONEq(t, `{
						"error": "must specify some form of authentication (either email+password, api-key, or jwt) in the request header or body",
						"error_code": "MissingParameter"
					}`, body)
				})
			})
		}
	})
}
