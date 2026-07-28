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

package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hanzoai/docdb/internal/util/must"
	"github.com/hanzoai/docdb/internal/util/testutil"
)

// initialize is the first request of the MCP protocol; it is answered by the
// server itself and never reaches a tool, so it needs no backend.
const initialize = `{
	"jsonrpc": "2.0",
	"id": 1,
	"method": "initialize",
	"params": {
		"protocolVersion": "2025-06-18",
		"capabilities": {},
		"clientInfo": {"name": "docdb-test", "version": "1"}
	}
}`

func TestRoutes(t *testing.T) {
	t.Parallel()

	lis := must.NotFail(Listen(&ListenOpts{TCPAddr: "127.0.0.1:0", L: testutil.Logger(t)}))
	t.Cleanup(func() {
		assert.NoError(t, lis.lis.Close())
	})

	app := lis.newApp(t.Context())

	do := func(t *testing.T, req *http.Request) (int, http.Header, string) {
		t.Helper()

		res, err := app.Fiber().Test(req)
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())

		return res.StatusCode, res.Header, string(body)
	}

	t.Run("Initialize", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initialize))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")

		code, header, body := do(t, req)

		assert.Equal(t, http.StatusOK, code)
		assert.NotEmpty(t, header.Get("Mcp-Session-Id"))
		assert.Contains(t, body, `"serverInfo"`)
		assert.Contains(t, body, `"DocDB"`)
	})

	t.Run("NotFound", func(t *testing.T) {
		code, _, body := do(t, httptest.NewRequest(http.MethodPost, "/no-such-path", nil))

		assert.Equal(t, http.StatusNotFound, code)
		assert.Equal(t, "404 page not found\n", body)
	})
}
