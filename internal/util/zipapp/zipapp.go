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

// Package zipapp is the single way DocDB builds and runs an HTTP router.
//
// Every DocDB listener (Data API, MCP, debug) declares its routes on a
// [zip.App] built by [New] and serves it on an already-bound [net.Listener]
// with [Serve]. Routing errors are rendered exactly as [net/http.ServeMux]
// renders them, so routes keep the status codes and bodies they had before
// the router moved to zip.
package zipapp

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"

	luxlog "github.com/luxfi/log"
	"github.com/zap-proto/fiber/v3"
	"github.com/zap-proto/zip"

	"github.com/hanzoai/docdb/internal/util/ctxutil"
	"github.com/hanzoai/docdb/internal/util/logging"
)

// New returns an app named name with no routes.
//
// The app is silent: DocDB logs listener lifecycle itself via [Serve],
// and zip's own logger would duplicate it in a different format.
func New(name string) *zip.App {
	return zip.New(zip.Config{
		AppName:               name,
		Logger:                luxlog.Noop(),
		DisableStartupMessage: true,
		ErrorHandler:          errorHandler,
	})
}

// BaseContext returns the middleware that makes ctx the parent of every
// request's context, the way [http.Server.BaseContext] did: a handler's context
// carries the listener's values and is canceled when the listener stops.
//
// It must be the first middleware an app uses, so that everything downstream
// derives from it.
func BaseContext(ctx context.Context) zip.Handler {
	return func(c *zip.Ctx) error {
		c.SetContext(ctx)

		return c.Next()
	}
}

// Text writes msg with the given status code exactly as [http.Error] does.
func Text(c *zip.Ctx, code int, msg string) error {
	return text(c.Fiber(), code, msg)
}

// text is [Text] on the underlying Fiber context, so [errorHandler] can share it.
func text(c fiber.Ctx, code int, msg string) error {
	c.Set(fiber.HeaderContentType, "text/plain; charset=utf-8")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Status(code)

	return c.SendString(msg + "\n")
}

// errorHandler renders an error the way net/http would.
//
// Unmatched paths and methods are the only errors DocDB handlers surface to the
// router (they write their own responses otherwise), and [http.ServeMux] answers
// those with plain text; anything else falls back to plain text too, never to a
// framework-shaped JSON body that no DocDB client expects.
func errorHandler(c fiber.Ctx, err error) error {
	code := http.StatusInternalServerError
	msg := err.Error()

	var fe *fiber.Error
	if errors.As(err, &fe) {
		code = fe.Code
		msg = fe.Message
	}

	switch code {
	case http.StatusNotFound:
		msg = "404 page not found"
	case http.StatusMethodNotAllowed:
		msg = "Method Not Allowed"
	}

	return text(c, code, msg)
}

// Serve runs app on the given already-bound listener until ctx is canceled.
//
// It exits when the handler is stopped and the listener closed.
// name is used for the "<name> server stopped" message.
func Serve(ctx context.Context, name string, app *zip.App, lis net.Listener, l *slog.Logger) {
	done := make(chan struct{})

	go func() {
		defer close(done)

		err := app.Fiber().Listener(lis, fiber.ListenConfig{DisableStartupMessage: true})
		if err != nil && !errors.Is(err, net.ErrClosed) {
			l.LogAttrs(ctx, logging.LevelDPanic, "Serve exited with unexpected error", logging.Error(err))
		}
	}()

	<-ctx.Done()

	// ctx is already canceled, but we want to inherit its values
	shutdownCtx, shutdownCancel := ctxutil.WithDelay(ctx)
	defer shutdownCancel(nil)

	if err := app.Fiber().ShutdownWithContext(shutdownCtx); err != nil && !errors.Is(err, fiber.ErrNotRunning) {
		l.LogAttrs(ctx, logging.LevelDPanic, "Shutdown exited with unexpected error", logging.Error(err))
	}

	<-done

	l.InfoContext(ctx, name+" server stopped")
}
