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

// Package docdb provides embeddable DocDB implementation.
//
// See [`build/version` package documentation]
// for information about Go build tags that affect this package.
//
// See [telemetry documentation] for basic anonymous usage data we collect.
// You can set [Config]'s Telemetry field to disable or explicitly enable it.
//
// [`build/version` package documentation]: https://pkg.go.dev/github.com/hanzoai/docdb/build/version
// [telemetry documentation]: https://docs.docdb.hanzo.ai/telemetry/
package docdb

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/hanzoai/docdb/build/version"
	"github.com/hanzoai/docdb/internal/handler/middleware"
	"github.com/hanzoai/docdb/internal/util/logging"
	"github.com/hanzoai/docdb/internal/util/setup"
	"github.com/hanzoai/docdb/internal/util/state"
	"github.com/hanzoai/docdb/internal/util/telemetry"
)

// Keep structure and order of Config in sync with the main package and documentation.
// Avoid breaking changes.

// Config represents DocDB configuration.
type Config struct {
	// PostgreSQL URL. Required.
	PostgreSQLURL string

	// Listen TCP address for MongoDB protocol.
	// If empty, TCP listener is disabled.
	ListenAddr string

	// State directory. Required.
	StateDir string

	// Defaults to [slog.LevelError].
	LogLevel slog.Leveler

	// Defaults to [io.Discard], effectively disabling logging.
	LogOutput io.Writer

	// Defaults to undecided.
	// Set to `true` to enable telemetry, `false` to disable it.
	// See https://docs.docdb.hanzo.ai/telemetry/.
	Telemetry *bool
}

// DocDB represents an instance of embedded DocDB implementation.
type DocDB struct {
	tr  *telemetry.Reporter
	res *setup.SetupResult
}

// New creates a new instance of embedded DocDB implementation.
func New(config *Config) (*DocDB, error) {
	version.Get().Package = "embedded"

	stateProvider, err := state.NewProviderDir(config.StateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to set up state provider: %w", err)
	}

	// Note that the current implementation requires `*logging.Handler` in the `getLog` command implementation.
	// TODO https://github.com/hanzoai/docdb/issues/4750

	logLevel := config.LogLevel
	if logLevel == nil {
		logLevel = slog.LevelError
	}

	logOutput := config.LogOutput
	if logOutput == nil {
		logOutput = io.Discard
	}

	lOpts := &logging.NewHandlerOpts{
		Base:       "console",
		Level:      logLevel,
		SkipChecks: true,
	}
	logger := logging.WithName(logging.Logger(logOutput, lOpts, ""), "docdb")

	mm := middleware.NewMetrics()

	tr, err := telemetry.NewReporter(&telemetry.NewReporterOpts{
		URL:            "https://beacon.docdb.hanzo.ai/",
		Dir:            config.StateDir,
		F:              telemetry.NewFlag(config.Telemetry),
		DNT:            os.Getenv("DO_NOT_TRACK"),
		ExecName:       os.Args[0],
		P:              stateProvider,
		Metrics:        mm,
		L:              logging.WithName(logger, "telemetry"),
		UndecidedDelay: time.Hour,
		ReportInterval: 24 * time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create telemetry reporter: %w", err)
	}

	//exhaustruct:enforce
	res := setup.Setup(context.TODO(), &setup.SetupOpts{
		Logger:        logger,
		StateProvider: stateProvider,
		Metrics:       mm,

		PostgreSQLURL:          config.PostgreSQLURL,
		Auth:                   false,
		ReplSetName:            "",
		SessionCleanupInterval: 0,

		ProxyAddr:        "",
		ProxyTLSCertFile: "",
		ProxyTLSKeyFile:  "",
		ProxyTLSCAFile:   "",

		TCPAddr:        config.ListenAddr,
		UnixAddr:       "",
		TLSAddr:        "",
		TLSCertFile:    "",
		TLSKeyFile:     "",
		TLSCAFile:      "",
		Mode:           middleware.NormalMode,
		TestRecordsDir: "",

		DataAPIAddr: "",

		MCPAddr: "",
	})
	if res == nil {
		return nil, fmt.Errorf("failed to create DocDB")
	}

	return &DocDB{
		tr:  tr,
		res: res,
	}, nil
}

// Run runs DocDB until ctx is canceled.
//
// When this method returns, all listeners, all client connections, and all PostgreSQL connections are closed.
//
// It is required to run this method in order to initialize listeners with their respective IP addresses and ports.
// Calling [*DocDB.MongoDBURI] before calling this method will result in a deadlock.
func (f *DocDB) Run(ctx context.Context) {
	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()
		f.tr.Run(ctx)
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()
		f.res.Run(ctx)
	}()

	wg.Wait()
}

// MongoDBURI returns MongoDB URI for this DocDB instance.
func (f *DocDB) MongoDBURI() string {
	u := &url.URL{
		Scheme: "mongodb",
		Host:   f.res.WireListener.TCPAddr().String(),
		Path:   "/",
	}

	return u.String()
}
