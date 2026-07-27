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

package server

import (
	"net/http"

	"github.com/AlekSi/lazyerrors"
	"github.com/FerretDB/wire/wirebson"
	"github.com/zap-proto/zip"

	"github.com/hanzoai/docdb/internal/dataapi/api"
	"github.com/hanzoai/docdb/internal/util/zipapp"
)

// DeleteMany implements [ServerInterface].
func (s *Server) DeleteMany(c *zip.Ctx) error {
	ctx := c.Context()

	var req api.DeleteRequestBody
	if err := s.decodeJSONRequest(c, &req); err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	deleteDoc, err := prepareDocument(
		"q", req.Filter,
		"limit", float64(0),
	)
	if err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	msg, err := prepareRequest(
		"delete", req.Collection,
		"$db", req.Database,
		"deletes", wirebson.MustArray(deleteDoc),
	)
	if err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	resp := s.m.Handle(ctx, msg)
	if resp == nil {
		return zipapp.Text(c, http.StatusInternalServerError, "internal error")
	}

	if !resp.OK() {
		return s.writeJSONError(c, resp)
	}

	res := api.DeleteResponseBody{
		DeletedCount: resp.Document().Get("n"),
	}

	return s.writeJSONResponse(c, &res)
}
