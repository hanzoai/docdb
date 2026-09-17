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
	"sync/atomic"
	"text/template"
	"time"

	"github.com/AlekSi/lazyerrors"
	"github.com/arl/statsviz"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
// [archiveHandler] fetches the other debug endpoints from the listener serving
// it, and finds that listener's address the way [http.Server] published it:
// under [http.LocalAddrContextKey] on the request context. The route puts it
// there, so the handler needs to know nothing about what is serving it.
func archive(l *slog.Logger) zip.Handler {
	h := zip.AdaptNetHTTP(archiveHandler(l))

	return func(c *zip.Ctx) error {
		fc := c.Fiber().RequestCtx()
		fc.SetUserValue(http.LocalAddrContextKey, fc.LocalAddr())

		return h(c)
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

	livez := promhttp.InstrumentHandlerDuration(
		probeDurations.MustCurryWith(prometheus.Labels{"probe": "livez"}),
		http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()

			if !opts.Livez(ctx) {
				l.Warn("Livez probe failed")
				rw.WriteHeader(http.StatusInternalServerError)

				return
			}

			l.Debug("Livez probe succeeded")
			rw.WriteHeader(http.StatusOK)
		}),
	)

	readyz := promhttp.InstrumentHandlerDuration(
		probeDurations.MustCurryWith(prometheus.Labels{"probe": "readyz"}),
		http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()

			if !opts.Livez(ctx) {
				l.Warn("Readyz probe failed - livez probe failed")
				rw.WriteHeader(http.StatusInternalServerError)

				return
			}

			if !opts.Readyz(ctx) {
				l.Warn("Readyz probe failed")
				rw.WriteHeader(http.StatusInternalServerError)

				return
			}

			l.Debug("Readyz probe succeeded")
			rw.WriteHeader(http.StatusOK)
		}),
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

	index := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Write(page.Bytes())
	})

	// The last resort for every path DocDB does not serve itself, and therefore
	// the fallback of [http.DefaultServeMux] rather than a route of its own.
	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, "/debug", http.StatusSeeOther)
	})

	app := zipapp.New("debug")

	app.Raw(zip.MethodAll, "/debug/metrics", zip.AdaptNetHTTP(metrics))
	app.Raw(zip.MethodAll, "/debug/archive", archive(l))
	app.Raw(zip.MethodAll, "/debug/archive.zip", func(c *zip.Ctx) error {
		return c.Redirect(http.StatusSeeOther, "/debug/archive")
	})
	app.Raw(zip.MethodAll, "/debug/livez", zip.AdaptNetHTTP(livez))
	app.Raw(zip.MethodAll, "/debug/readyz", zip.AdaptNetHTTP(readyz))
	app.Raw(zip.MethodAll, "/debug", zip.AdaptNetHTTP(index))

	// Everything else is served by [http.DefaultServeMux]: the runtime's own
	// pprof, expvar and trace endpoints, statsviz, and the "/" redirect above.
	// None of them are DocDB's, so they are fronted whole instead of rewritten;
	// the adapter forwards flushes and connection hijacks, which is what
	// statsviz's WebSocket needs.
	app.Raw(zip.MethodAll, "/*", zip.AdaptNetHTTP(http.DefaultServeMux))

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
