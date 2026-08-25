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

// Package debug provides debug facilities.
package debug

import (
	"bytes"
	"context"
	_ "expvar" // for metrics
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	_ "net/http/pprof" // for profiling
	"slices"
	"strconv"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/AlekSi/lazyerrors"
	"github.com/arl/statsviz"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/zap-proto/fiber/v3"
	"github.com/zap-proto/zip"
	_ "golang.org/x/net/trace"

	"github.com/hanzoai/docdb/internal/util/must"
	"github.com/hanzoai/docdb/internal/util/zipapp"
)

// Parts of Prometheus metric names.
const (
	// TODO https://github.com/hanzoai/docdb/issues/3420
	namespace = "docdb"
	subsystem = "debug"
)

// setup ensures that debug handler is set up only once.
var setup atomic.Bool

// archive returns the debug archive route.
//
// [archiveFile] reads the endpoints it cannot produce itself back from the
// listener serving this request, whose address fasthttp already holds.
func archive(l *slog.Logger) zip.Handler {
	return func(c *zip.Ctx) error {
		fc := c.Fiber()

		// net/http's transport touches an outbound request's context after the
		// response is read, and fasthttp reuses its own context for the next
		// request on the connection as soon as this handler returns. The reads
		// get a context that keeps the request's values and ends with the
		// handler instead.
		ctx, cancel := context.WithCancel(context.WithoutCancel(c.Context()))
		defer cancel()

		name, body := archiveFile(ctx, l, fc.RequestCtx().LocalAddr())

		fc.Set(fiber.HeaderContentType, "application/zip")
		fc.Set(fiber.HeaderContentDisposition, "attachment; filename="+name)
		fc.Status(http.StatusOK)

		return fc.Send(body)
	}
}

// redirect returns a route that answers with code and location the way
// [http.Redirect] does: a one-line HTML body for GET, the same Content-Type but
// no body for HEAD, and neither for any other method.
func redirect(location string, code int) zip.Handler {
	body := fmt.Sprintf("<a href=%q>%s</a>.\n\n", location, http.StatusText(code))

	return func(c *zip.Ctx) error {
		fc := c.Fiber()
		fc.Set(fiber.HeaderLocation, location)

		switch fc.Method() {
		case fiber.MethodGet:
			fc.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
			fc.Status(code)

			return fc.SendString(body)

		case fiber.MethodHead:
			fc.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		}

		fc.Status(code)

		return nil
	}
}

// probe returns a route that answers 200 when check passes and 500 when it does
// not, with an empty body either way, and observes how long check took on obs
// under the code it answered.
//
// Currying obs with the probe's name leaves "code" as the one label this route
// fills, which is what the histogram in [Listen] is partitioned by.
func probe(obs prometheus.ObserverVec, check func(context.Context) bool) zip.Handler {
	return func(c *zip.Ctx) error {
		start := time.Now()

		ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
		defer cancel()

		code := http.StatusInternalServerError
		if check(ctx) {
			code = http.StatusOK
		}

		obs.With(prometheus.Labels{"code": strconv.Itoa(code)}).Observe(time.Since(start).Seconds())

		c.Fiber().Status(code)

		return nil
	}
}

// Probe should return true on success and false on failure (or context cancellation).
// It may log additional information if needed.
//
// It must be thread-safe.
type Probe func(ctx context.Context) bool

// Listener represents TCP listener with debug HTTP handler.
//
//nolint:vet // for readability
type Listener struct {
	opts     *ListenOpts
	lis      net.Listener
	app      *zip.App
	handlers map[string]string
}

// ListenOpts represents [Listen] options.
//
//nolint:vet // for readability
type ListenOpts struct {
	TCPAddr string
	L       *slog.Logger
	R       prometheus.Registerer
	Livez   Probe
	Readyz  Probe
}

// Listen creates a new debug handler and starts listener on the given TCP address.
//
// This function can be called only once because it affects [http.DefaultServeMux].
func Listen(opts *ListenOpts) (*Listener, error) {
	if setup.Swap(true) {
		panic("debug handler is already set up")
	}

	must.NotBeZero(opts)

	l := opts.L

	stdL := slog.NewLogLogger(l.Handler(), slog.LevelError)

	probeDurations := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "probe_response_seconds",
			Help:      "Probe response time seconds.",
			Buckets:   []float64{0.1, 0.5, 1, 5},
		},
		[]string{"probe", "code"},
	)

	opts.R.MustRegister(probeDurations)

	metrics := promhttp.InstrumentMetricHandler(
		opts.R, promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{
			ErrorLog:          stdL,
			ErrorHandling:     promhttp.ContinueOnError,
			Registry:          opts.R,
			EnableOpenMetrics: true,
		}),
	)

	livez := probe(
		probeDurations.MustCurryWith(prometheus.Labels{"probe": "livez"}),
		func(ctx context.Context) bool {
			if !opts.Livez(ctx) {
				l.Warn("Livez probe failed")

				return false
			}

			l.Debug("Livez probe succeeded")

			return true
		},
	)

	readyz := probe(
		probeDurations.MustCurryWith(prometheus.Labels{"probe": "readyz"}),
		func(ctx context.Context) bool {
			if !opts.Livez(ctx) {
				l.Warn("Readyz probe failed - livez probe failed")

				return false
			}

			if !opts.Readyz(ctx) {
				l.Warn("Readyz probe failed")

				return false
			}

			l.Debug("Readyz probe succeeded")

			return true
		},
	)

	svOpts := []statsviz.Option{
		statsviz.Root("/debug/graphs"),
		// TODO https://github.com/hanzoai/docdb/issues/3600
	}
	must.NoError(statsviz.Register(http.DefaultServeMux, svOpts...))

	handlers := map[string]string{
		// custom handlers registered above
		"/debug/metrics": "Metrics in Prometheus format",
		"/debug/archive": "Zip archive with debugging information",
		"/debug/graphs":  "Visualize metrics",
		"/debug/livez":   "Liveness probe",
		"/debug/readyz":  "Readiness probe",

		// stdlib handlers
		"/debug/vars":     "Expvar package metrics",
		"/debug/pprof":    "Runtime profiling data for pprof",
		"/debug/requests": "/x/net/trace requests",
		"/debug/events":   "/x/net/trace events",
	}

	var page bytes.Buffer
	must.NoError(template.Must(template.New("debug").Parse(`
	<html>
	<body>
	<ul>
	{{range $path, $desc := .}}
		<li><a href="{{$path}}">{{$path}}</a>: {{$desc}}</li>
	{{end}}
	</ul>
	</body>
	</html>
	`)).Execute(&page, handlers))

	// page is written once here and only read from now on, so every request
	// answers with the same bytes without copying them.
	index := func(c *zip.Ctx) error {
		fc := c.Fiber()
		fc.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		fc.Status(http.StatusOK)

		return fc.Send(page.Bytes())
	}

	// The last resort for every path DocDB does not serve itself, and therefore
	// the fallback of [http.DefaultServeMux] rather than a route of its own.
	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, "/debug", http.StatusSeeOther)
	})

	app := zipapp.New("debug")

	// promhttp hands out an [http.Handler] and nothing else - HandlerFor,
	// HandlerForTransactional and InstrumentMetricHandler all return one, and
	// exposition, content negotiation and the handler's own request counters
	// live inside it. So this route is fronted rather than rewritten.
	app.All("/debug/metrics", zip.AdaptNetHTTP(metrics))

	app.All("/debug/archive", archive(l))
	app.All("/debug/archive.zip", redirect("/debug/archive", http.StatusSeeOther))
	app.All("/debug/livez", livez)
	app.All("/debug/readyz", readyz)
	app.All("/debug", index)

	// Everything else is served by [http.DefaultServeMux]: the runtime's own
	// pprof, expvar and trace endpoints, statsviz, and the "/" redirect above.
	// None of them are DocDB's, and none is reachable except as an
	// [http.Handler], so they are fronted whole instead of rewritten; the
	// adapter forwards flushes and connection hijacks, which is what statsviz's
	// WebSocket needs.
	//
	// The mux also cleans the request path and answers 307 for the cleaned form
	// before it matches anything, which the zip router does not do. It is
	// therefore the only thing answering "/debug/../x" and "//x" here.
	app.All("/*", zip.AdaptNetHTTP(http.DefaultServeMux))

	lis, err := net.Listen("tcp", opts.TCPAddr)
	if err != nil {
		return nil, lazyerrors.Error(err)
	}

	return &Listener{
		opts:     opts,
		lis:      lis,
		app:      app,
		handlers: handlers,
	}, nil
}

// Run runs debug handler until ctx is canceled.
//
// It exits when handler is stopped and listener closed.
func (lis *Listener) Run(ctx context.Context) {
	l := lis.opts.L

	root := fmt.Sprintf("http://%s", lis.lis.Addr())

	l.InfoContext(ctx, fmt.Sprintf("Starting debug server on %s/debug", root))

	paths := slices.Sorted(maps.Keys(lis.handlers))

	for _, path := range paths {
		l.InfoContext(ctx, fmt.Sprintf("%s%s - %s", root, path, lis.handlers[path]))
	}

	zipapp.Serve(ctx, "Debug", lis.app, lis.lis, l)
}
