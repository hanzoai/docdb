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
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/AlekSi/lazyerrors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/zap-proto/zip"

	"github.com/hanzoai/docdb/build/version"
	"github.com/hanzoai/docdb/internal/clientconn/conninfo"
	"github.com/hanzoai/docdb/internal/handler/middleware"
	"github.com/hanzoai/docdb/internal/util/zipapp"
)

// Listener represents MCP listener.
type Listener struct {
	opts *ListenOpts
	lis  net.Listener
	srv  *server
}

// ListenOpts represents [Listen] options.
type ListenOpts struct { //nolint:vet // for readability
	L       *slog.Logger
	M       *middleware.Middleware
	TCPAddr string

	// TODO https://github.com/hanzoai/docdb/issues/5309
	// Auth bool
}

// Listen creates a new MCP handler and starts listener on the given TCP address.
// [Listener.Run] must be called on the returned value.
func Listen(opts *ListenOpts) (*Listener, error) {
	lis, err := net.Listen("tcp", opts.TCPAddr)
	if err != nil {
		return nil, lazyerrors.Error(err)
	}

	return &Listener{
		opts: opts,
		lis:  lis,
		srv:  newServer(opts.L, opts.M),
	}, nil
}

// newApp returns the MCP router.
func (lis *Listener) newApp(ctx context.Context) *zip.App {
	s := mcp.NewServer(&mcp.Implementation{Name: "DocDB", Version: version.Get().Version}, nil)
	lis.srv.addTools(s)

	mcpHandler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server { return s }, nil)

	app := zipapp.New("mcp")
	app.Use(zipapp.BaseContext(ctx))

	// The MCP SDK's streamable HTTP handler owns the whole request/response
	// lifecycle - session headers, SSE streams, flushing - so it is fronted
	// as-is instead of being reimplemented.
	// TODO https://github.com/hanzoai/docdb/issues/5309
	app.All("/mcp", zip.AdaptNetHTTP(connInfoMiddleware(mcpHandler)))

	return app
}

// Run runs MCP handler until ctx is canceled.
//
// It exits when handler is stopped and listener closed.
func (lis *Listener) Run(ctx context.Context) {
	lis.opts.L.InfoContext(ctx, fmt.Sprintf("Starting MCP server on http://%s/mcp", lis.lis.Addr()))

	zipapp.Serve(ctx, "MCP", lis.newApp(ctx), lis.lis, lis.opts.L)
}

// connInfoMiddleware returns a handler function that creates a new [*conninfo.ConnInfo],
// calls the next handler, and closes the connection info after the request is done.
func connInfoMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connInfo := conninfo.New()
		defer connInfo.Close()
		next.ServeHTTP(w, r.WithContext(conninfo.Ctx(r.Context(), connInfo)))
	})
}

// Addr returns TCP listener's address.
// It can be used to determine an actually used port, if it was zero.
func (lis *Listener) Addr() net.Addr {
	return lis.lis.Addr()
}

// Describe implements [prometheus.Collector].
func (lis *Listener) Describe(ch chan<- *prometheus.Desc) {
}

// Collect implements [prometheus.Collector].
func (lis *Listener) Collect(ch chan<- prometheus.Metric) {
}
