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

// Package dataapi provides a Data API wrapper,
// which allows DocDB to be used over HTTP instead of MongoDB wire protocol.
package dataapi

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/AlekSi/lazyerrors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/zap-proto/zip"

	"github.com/hanzoai/docdb/internal/dataapi/server"
	"github.com/hanzoai/docdb/internal/handler/middleware"
	"github.com/hanzoai/docdb/internal/util/zipapp"
)

// Listener represents Data API TCP listener and HTTP handler.
type Listener struct {
	opts *ListenOpts
	lis  net.Listener
}

// ListenOpts represents [Listen] options.
type ListenOpts struct {
	L       *slog.Logger
	M       *middleware.Middleware
	TCPAddr string
	Auth    bool
}

// Listen creates a new Data API handler and starts listener on the given TCP address.
// [Listener.Run] must be called on the returned value.
func Listen(opts *ListenOpts) (*Listener, error) {
	lis, err := net.Listen("tcp", opts.TCPAddr)
	if err != nil {
		return nil, lazyerrors.Error(err)
	}

	return &Listener{
		opts: opts,
		lis:  lis,
	}, nil
}

// newApp returns the Data API router: nine actions described by the OpenAPI
// specification, plus the specification itself.
//
// Requests pass through connection info and, when authentication is enabled,
// SCRAM authentication - including requests for the specification and requests
// that match no route at all, exactly as when both were [http.Handler] wrappers
// around the whole mux.
func newApp(ctx context.Context, opts *ListenOpts) *zip.App {
	s := server.New(opts.L, opts.M)

	app := zipapp.New("dataapi")

	app.Use(zipapp.BaseContext(ctx))
	// zip.H because a method value is a bare func, not the named Handler type
	// Use's Component set is closed over. BaseContext needs no wrapper: it is
	// declared as returning a zip.Handler already.
	app.Use(zip.H(s.ConnInfo))

	if opts.Auth {
		app.Use(zip.H(s.Auth))
	}

	app.Get("/openapi.json", s.OpenAPISpec)

	action := app.Group("/action")
	action.Post("/aggregate", s.Aggregate)
	action.Post("/deleteMany", s.DeleteMany)
	action.Post("/deleteOne", s.DeleteOne)
	action.Post("/find", s.Find)
	action.Post("/findOne", s.FindOne)
	action.Post("/insertMany", s.InsertMany)
	action.Post("/insertOne", s.InsertOne)
	action.Post("/updateMany", s.UpdateMany)
	action.Post("/updateOne", s.UpdateOne)

	return app
}

// Run runs Data API handler until ctx is canceled.
//
// It exits when handler is stopped and listener closed.
func (lis *Listener) Run(ctx context.Context) {
	l := lis.opts.L

	l.InfoContext(ctx, fmt.Sprintf("Starting Data API server on http://%s/", lis.Addr()))
	l.InfoContext(ctx, fmt.Sprintf("http://%s/openapi.json - OpenAPI spec", lis.Addr()))

	zipapp.Serve(ctx, "Data API", newApp(ctx, lis.opts), lis.lis, l)
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
