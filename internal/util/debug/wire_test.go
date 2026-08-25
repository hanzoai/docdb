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

package debug

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkedHeaders are asserted on every row of the probe table, so a row that
// says nothing about one of them is asserting that it is absent.
var checkedHeaders = []string{
	"Content-Type",
	"X-Content-Type-Options",
	"Location",
	"Content-Disposition",
	"Allow",
	"Content-Length",
}

// wire is one row of the probe table: a request, and the answer the debug
// listener owes it byte for byte.
type wire struct {
	// header holds the exact value of each of [checkedHeaders]; one this map
	// does not name must be absent from the response.
	header map[string]string

	// prefix holds headers whose value varies between runs, asserted as a
	// leading substring. An empty prefix asserts nothing beyond presence.
	prefix map[string]string

	method string
	target string

	// body is the exact response body when exact is set. Otherwise it is the
	// body's leading bytes, and an empty body asserts only that some body came
	// back.
	body string

	code  int
	exact bool
}

// html is the Content-Type net/http and the index route both answer with.
const html = "text/html; charset=utf-8"

// rawGet sends method and target verbatim, so a request-target no URL parser
// would keep ("/debug/../x") reaches the router exactly as written.
func rawGet(t *testing.T, addr, method, target string) (*http.Response, string) {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	defer conn.Close() //nolint:errcheck // the response is already read

	_, err = fmt.Fprintf(conn, "%s %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", method, target, addr)
	require.NoError(t, err)

	res, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)

	b, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())

	return res, string(b)
}

// check runs one row and asserts the answer matches it.
func check(t *testing.T, addr string, w wire) {
	t.Helper()

	res, body := rawGet(t, addr, w.method, w.target)

	assert.Equal(t, w.code, res.StatusCode)

	for _, h := range checkedHeaders {
		got := res.Header.Get(h)

		if p, ok := w.prefix[h]; ok {
			assert.Contains(t, res.Header, http.CanonicalHeaderKey(h), h)
			assert.True(t, strings.HasPrefix(got, p), "%s: %q has no prefix %q", h, got, p)

			continue
		}

		assert.Equal(t, w.header[h], got, h)
	}

	switch {
	case w.exact:
		assert.Equal(t, w.body, body)
	case w.body != "":
		assert.True(t, strings.HasPrefix(body, w.body), "body %q has no prefix %q", truncate(body), w.body)
	default:
		assert.NotEmpty(t, body)
	}
}

// truncate shortens a body so a failure message stays readable.
func truncate(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}

	return s[:max] + "..."
}

// indexBody renders the body the index route answers with: the template in [Listen]
// over handlers, whose keys a Go template walks in sorted order.
func indexBody(handlers map[string]string) string {
	var b strings.Builder

	b.WriteString("\n\t<html>\n\t<body>\n\t<ul>\n\t")

	for _, path := range slices.Sorted(maps.Keys(handlers)) {
		fmt.Fprintf(&b, "\n\t\t<li><a href=%q>%s</a>: %s</li>\n\t", path, path, handlers[path])
	}

	b.WriteString("\n\t</ul>\n\t</body>\n\t</html>\n\t")

	return b.String()
}

// probeCount returns how many times the probe histogram observed probe with
// status code, and zero when that pair has never been observed.
func probeCount(t *testing.T, g prometheus.Gatherer, probe, code string) uint64 {
	t.Helper()

	families, err := g.Gather()
	require.NoError(t, err)

	for _, f := range families {
		if f.GetName() != "docdb_debug_probe_response_seconds" {
			continue
		}

		for _, m := range f.GetMetric() {
			want := map[string]string{"probe": probe, "code": code}
			got := map[string]string{}

			for _, l := range m.GetLabel() {
				got[l.GetName()] = l.GetValue()
			}

			require.Equal(t, []string{"code", "probe"}, slices.Sorted(maps.Keys(got)), "probe histogram labels")

			if maps.Equal(want, got) {
				require.NotNil(t, m.GetHistogram(), "probe metric is a histogram")

				return m.GetHistogram().GetSampleCount()
			}
		}
	}

	return 0
}

// probeWire runs one probe row and asserts it also recorded exactly one
// observation on the histogram, under the probe's name and the answered code.
func probeWire(t *testing.T, addr string, g prometheus.Gatherer, probe string, w wire) {
	t.Helper()

	code := fmt.Sprint(w.code)
	before := probeCount(t, g, probe, code)

	check(t, addr, w)

	assert.Equal(t, before+1, probeCount(t, g, probe, code), "%s observations for code %s", probe, code)
}

// testWire asserts the answer of every address the debug listener serves.
//
// The table was captured from the routes as they stood when each was an
// adapted net/http handler, and holds them to it now that they are not. Rows
// whose answer comes from [http.DefaultServeMux] are here for the same reason:
// the catch-all is what cleans "/debug/../x", and dropping it would silently
// turn three redirects into something else.
func testWire(t *testing.T, addr string, reg *prometheus.Registry, handlers map[string]string, livez, readyz *atomic.Bool) {
	livez.Store(true)
	readyz.Store(true)

	idx := indexBody(handlers)

	for _, w := range []wire{{
		method: "GET",
		target: "/debug/archive.zip",
		code:   http.StatusSeeOther,
		header: map[string]string{"Content-Type": html, "Location": "/debug/archive", "Content-Length": "41"},
		body:   "<a href=\"/debug/archive\">See Other</a>.\n\n",
		exact:  true,
	}, {
		// net/http sends the redirect's Content-Type but no body for HEAD, and
		// neither for any other method.
		method: "HEAD",
		target: "/debug/archive.zip",
		code:   http.StatusSeeOther,
		header: map[string]string{"Content-Type": html, "Location": "/debug/archive"},
		exact:  true,
	}, {
		method: "POST",
		target: "/debug/archive.zip",
		code:   http.StatusSeeOther,
		header: map[string]string{"Location": "/debug/archive", "Content-Length": "0"},
		exact:  true,
	}, {
		method: "GET",
		target: "/debug",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": html, "Content-Length": "770"},
		body:   idx,
		exact:  true,
	}, {
		// The index route takes any method, and a trailing slash still reaches it.
		method: "POST",
		target: "/debug",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": html, "Content-Length": "770"},
		body:   idx,
		exact:  true,
	}, {
		method: "PATCH",
		target: "/debug",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": html, "Content-Length": "770"},
		body:   idx,
		exact:  true,
	}, {
		method: "GET",
		target: "/debug/",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": html, "Content-Length": "770"},
		body:   idx,
		exact:  true,
	}, {
		method: "GET",
		target: "/debug/metrics",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": "text/plain; version=0.0.4; charset=utf-8; escaping=underscores"},
		prefix: map[string]string{"Content-Length": ""},
		body:   "# HELP go_gc_duration_seconds ",
	}, {
		method: "GET",
		target: "/debug/archive",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": "application/zip"},
		prefix: map[string]string{"Content-Disposition": "attachment; filename=docdb-", "Content-Length": ""},
		body:   "PK\x03\x04",
	}, {
		// Handlers of other packages, behind the catch-all on [http.DefaultServeMux].
		method: "GET",
		target: "/debug/vars",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": "application/json; charset=utf-8"},
		prefix: map[string]string{"Content-Length": ""},
		body:   "{\n\"cmdline\": [",
	}, {
		method: "GET",
		target: "/debug/pprof/",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": html, "X-Content-Type-Options": "nosniff"},
		prefix: map[string]string{"Content-Length": ""},
		body:   "<html>\n<head>\n<title>/debug/pprof/</title>",
	}, {
		method: "GET",
		target: "/debug/pprof/cmdline",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": "text/plain; charset=utf-8", "X-Content-Type-Options": "nosniff"},
		prefix: map[string]string{"Content-Length": ""},
	}, {
		method: "GET",
		target: "/debug/pprof/heap?debug=1",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": "text/plain; charset=utf-8", "X-Content-Type-Options": "nosniff"},
		prefix: map[string]string{"Content-Length": ""},
		body:   "heap profile: ",
	}, {
		// The pprof patterns are registered for GET only, but "/" matches every
		// method, so a POST reaches the fallback redirect rather than a 405.
		method: "POST",
		target: "/debug/pprof/",
		code:   http.StatusSeeOther,
		header: map[string]string{"Location": "/debug", "Content-Length": "0"},
		exact:  true,
	}, {
		method: "GET",
		target: "/debug/requests",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": html},
		prefix: map[string]string{"Content-Length": ""},
		body:   "\n\n<html>\n\t<head>\n\t<title>/debug/requests</title>",
	}, {
		method: "GET",
		target: "/debug/events",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": html},
		prefix: map[string]string{"Content-Length": ""},
		body:   "\n<html>\n\t<head>\n\t\t<title>events</title>",
	}, {
		method: "GET",
		target: "/debug/graphs/",
		code:   http.StatusOK,
		header: map[string]string{"Content-Type": html},
		prefix: map[string]string{"Content-Length": ""},
		body:   "<!DOCTYPE html>\n<html lang=\"en\">",
	}, {
		// Everything DocDB does not serve itself lands on the fallback.
		method: "GET",
		target: "/",
		code:   http.StatusSeeOther,
		header: map[string]string{"Content-Type": html, "Location": "/debug", "Content-Length": "33"},
		body:   "<a href=\"/debug\">See Other</a>.\n\n",
		exact:  true,
	}, {
		method: "GET",
		target: "/no-such-path",
		code:   http.StatusSeeOther,
		header: map[string]string{"Content-Type": html, "Location": "/debug", "Content-Length": "33"},
		body:   "<a href=\"/debug\">See Other</a>.\n\n",
		exact:  true,
	}, {
		// The zip router matches paths as sent. Only [http.ServeMux] cleans
		// them, and it answers 307 for the cleaned form before matching, so
		// these three are the catch-all's answers and not the routes' own.
		method: "GET",
		target: "/debug/../x",
		code:   http.StatusTemporaryRedirect,
		header: map[string]string{"Content-Type": html, "Location": "/x", "Content-Length": "38"},
		body:   "<a href=\"/x\">Temporary Redirect</a>.\n\n",
		exact:  true,
	}, {
		method: "GET",
		target: "//x",
		code:   http.StatusTemporaryRedirect,
		header: map[string]string{"Content-Type": html, "Location": "/x", "Content-Length": "38"},
		body:   "<a href=\"/x\">Temporary Redirect</a>.\n\n",
		exact:  true,
	}, {
		// A dot segment does not reach /debug/metrics; it is cleaned first.
		method: "GET",
		target: "/debug/./metrics",
		code:   http.StatusTemporaryRedirect,
		header: map[string]string{"Content-Type": html, "Location": "/debug/metrics", "Content-Length": "50"},
		body:   "<a href=\"/debug/metrics\">Temporary Redirect</a>.\n\n",
		exact:  true,
	}} {
		t.Run(w.method+" "+w.target, func(t *testing.T) {
			check(t, addr, w)
		})
	}

	// The probes answer with a status and nothing else, and each answer is one
	// observation on the histogram under that status.
	ok := wire{
		method: "GET",
		target: "/debug/livez",
		code:   http.StatusOK,
		header: map[string]string{"Content-Length": "0"},
		exact:  true,
	}

	t.Run("GET /debug/livez", func(t *testing.T) {
		probeWire(t, addr, reg, "livez", ok)
	})

	t.Run("POST /debug/livez", func(t *testing.T) {
		post := ok
		post.method = "POST"
		probeWire(t, addr, reg, "livez", post)
	})

	t.Run("GET /debug/readyz", func(t *testing.T) {
		ready := ok
		ready.target = "/debug/readyz"
		probeWire(t, addr, reg, "readyz", ready)
	})

	t.Run("GET /debug/livez failing", func(t *testing.T) {
		livez.Store(false)
		defer livez.Store(true)

		down := ok
		down.code = http.StatusInternalServerError
		probeWire(t, addr, reg, "livez", down)
	})

	t.Run("GET /debug/readyz failing", func(t *testing.T) {
		readyz.Store(false)
		defer readyz.Store(true)

		down := ok
		down.target = "/debug/readyz"
		down.code = http.StatusInternalServerError
		probeWire(t, addr, reg, "readyz", down)
	})
}
