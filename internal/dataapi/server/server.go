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

// Package server provides a Data API server handlers.
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/AlekSi/lazyerrors"
	"github.com/FerretDB/wire/wirebson"
	"github.com/xdg-go/scram"
	"github.com/zap-proto/zip"

	"github.com/hanzoai/docdb/internal/clientconn/conninfo"
	"github.com/hanzoai/docdb/internal/dataapi/api"
	"github.com/hanzoai/docdb/internal/handler/middleware"
	"github.com/hanzoai/docdb/internal/util/logging"
	"github.com/hanzoai/docdb/internal/util/must"
	"github.com/hanzoai/docdb/internal/util/zipapp"
)

// New creates a new Server.
func New(l *slog.Logger, handler *middleware.Middleware) *Server {
	return &Server{
		l: l,
		m: handler,
	}
}

// Server implements services described by OpenAPI description file.
type Server struct {
	l *slog.Logger
	m *middleware.Middleware
}

// Auth handles SCRAM authentication based on the username and password specified in request.
// After a successful handshake it calls the next handler.
func (s *Server) Auth(c *zip.Ctx) error {
	ctx := c.Context()
	username, password, ok := basicAuth(c)

	if !ok {
		return writeError(c, errorNoAuthenticationSpecified, http.StatusBadRequest)
	}

	if password == "" || username == "" {
		return writeError(c, errorMissingAuthenticationParameter, http.StatusBadRequest)
	}

	client, err := scram.SHA256.NewClient(username, password, "")
	if err != nil {
		return zipapp.Text(c, http.StatusBadRequest, lazyerrors.Error(err).Error())
	}

	conv := client.NewConversation()

	payload, err := conv.Step("")
	if err != nil {
		return zipapp.Text(c, http.StatusBadRequest, lazyerrors.Error(err).Error())
	}

	msg := must.NotFail(prepareRequest(
		"saslStart", int32(1),
		"mechanism", "SCRAM-SHA-256",
		"payload", wirebson.Binary{B: []byte(payload)},
		// use skipEmptyExchange to complete the handshake with one `saslStart` and one `saslContinue`
		"options", wirebson.MustDocument("skipEmptyExchange", true),
		"$db", "admin",
	))

	resp := s.m.Handle(ctx, msg)
	if resp == nil {
		return zipapp.Text(c, http.StatusInternalServerError, "internal error")
	}

	if !resp.OK() {
		return s.writeJSONError(c, resp)
	}

	convID := resp.Document().Get("conversationId").(int32)
	payloadBytes := resp.Document().Get("payload").(wirebson.Binary).B

	payload, err = conv.Step(string(payloadBytes))
	if err != nil {
		return zipapp.Text(c, http.StatusUnauthorized, lazyerrors.Error(err).Error())
	}

	msg = must.NotFail(prepareRequest(
		"saslContinue", int32(1),
		"conversationId", convID,
		"payload", wirebson.Binary{B: []byte(payload)},
		"$db", "admin",
	))

	resp = s.m.Handle(ctx, msg)
	if resp == nil {
		return zipapp.Text(c, http.StatusInternalServerError, "internal error")
	}

	if !resp.OK() {
		return s.writeJSONError(c, resp)
	}

	if !resp.Document().Get("done").(bool) {
		return zipapp.Text(c, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
	}

	payloadBytes = resp.Document().Get("payload").(wirebson.Binary).B

	if _, err = conv.Step(string(payloadBytes)); err != nil {
		return zipapp.Text(c, http.StatusUnauthorized, lazyerrors.Error(err).Error())
	}

	if !conv.Valid() {
		return zipapp.Text(c, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
	}

	return c.Next()
}

// basicAuth returns the username and password from the request's HTTP Basic
// Authentication header, exactly as [http.Request.BasicAuth] parses it.
func basicAuth(c *zip.Ctx) (username, password string, ok bool) {
	r := http.Request{Header: http.Header{"Authorization": []string{c.Header("Authorization")}}}

	return r.BasicAuth()
}

// ConnInfo creates a new [*conninfo.ConnInfo], calls the next handler,
// and closes the connection info after the request is done.
func (s *Server) ConnInfo(c *zip.Ctx) error {
	ci := conninfo.New()

	defer ci.Close()

	c.SetContext(conninfo.Ctx(c.Context(), ci))

	return c.Next()
}

// writeJSONResponse marshals provided res document into extended JSON and writes it as the response body.
func (s *Server) writeJSONResponse(c *zip.Ctx, res api.Response) error {
	ctx := c.Context()

	c.SetHeader("Content-Type", "application/json")

	buf := new(bytes.Buffer)

	if err := json.NewEncoder(buf).Encode(res); err != nil {
		s.l.ErrorContext(ctx, "marshalJSON failed", logging.Error(err))
	}

	if s.l.Enabled(ctx, slog.LevelDebug) {
		// extended JSON value writer always finish with '\n' character
		s.l.DebugContext(ctx, fmt.Sprintf("Results:\n%s", strings.TrimSpace(buf.String())))
	}

	return c.Fiber().Send(buf.Bytes())
}

// TODO https://github.com/hanzoai/docdb/issues/4965
func (s *Server) writeJSONError(c *zip.Ctx, resp *middleware.Response) error {
	doc := resp.Document()
	errmsg := doc.Get("errmsg").(string)
	codeName := doc.Get("codeName").(string)

	c.Status(http.StatusInternalServerError)

	return s.writeJSONResponse(c, &api.Error{
		Error:     errmsg,
		ErrorCode: codeName,
	})
}

// prepareDocument creates a new bson document from the given pairs of
// field names and values, which can be used as handler command msg.
//
// If any of pair values is nil it's ignored.
func prepareDocument(pairs ...any) (*wirebson.Document, error) {
	l := len(pairs)

	if l%2 != 0 {
		return nil, lazyerrors.Errorf("invalid number of arguments: %d", l)
	}

	docPairs := make([]any, 0, l)

	for i := 0; i < l; i += 2 {
		var err error

		key := pairs[i]
		v := pairs[i+1]

		switch val := v.(type) {
		// json.RawMessage is the non-pointer exception.
		// Other non-pointer types don't need special handling.
		case json.RawMessage:
			v, err = unmarshalSingleJSON(&val)
			if err != nil {
				return nil, err
			}

		case *json.RawMessage:
			if val == nil {
				continue
			}

			v, err = unmarshalSingleJSON(val)
			if err != nil {
				return nil, err
			}
		case *float32:
			if val == nil {
				continue
			}

			v = float64(*val)
		case *bool:
			if val == nil {
				continue
			}

			v = *val
		}

		if v == nil {
			continue
		}

		docPairs = append(docPairs, key, v)
	}

	return wirebson.NewDocument(docPairs...)
}

// prepareRequest creates a new middleware request from the given pairs of field names and values,
// which can be used as handler command msg.
//
// If any of pair values is nil it's ignored.
func prepareRequest(pairs ...any) (*middleware.Request, error) {
	doc, err := prepareDocument(pairs...)
	if err != nil {
		return nil, lazyerrors.Error(err)
	}

	return middleware.RequestDoc(doc)
}

// decodeJSONRequest logs the request when debug logging is enabled, then decodes
// its JSON body into the provided oapi generated request struct.
func (s *Server) decodeJSONRequest(c *zip.Ctx, out any) error {
	if s.l.Enabled(c.Context(), slog.LevelDebug) {
		s.l.DebugContext(c.Context(), fmt.Sprintf("Request:\n%s", c.Fiber().Request().String()))
	}

	if !strings.HasPrefix(c.Header("Content-Type"), "application/json") {
		return lazyerrors.New("Content-Type must be set to application/json")
	}

	if err := json.NewDecoder(bytes.NewReader(c.Body())).Decode(&out); err != nil {
		return lazyerrors.Error(err)
	}

	return nil
}
