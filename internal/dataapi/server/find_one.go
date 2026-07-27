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
	"github.com/hanzoai/docdb/internal/util/must"
	"github.com/hanzoai/docdb/internal/util/zipapp"
)

// FindOne implements [ServerInterface].
func (s *Server) FindOne(c *zip.Ctx) error {
	ctx := c.Context()

	var req api.FindOneRequestBody
	if err := s.decodeJSONRequest(c, &req); err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	msg, err := prepareRequest(
		"find", req.Collection,
		"$db", req.Database,
		"filter", req.Filter,
		"projection", req.Projection,
		"limit", float64(1),
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

	cursor := resp.Document().Get("cursor").(wirebson.AnyDocument)
	firstBatch := must.NotFail(cursor.Decode()).Get("firstBatch").(wirebson.AnyArray)

	var res api.FindOneResponseBody

	docs := must.NotFail(firstBatch.Decode())
	if docs.Len() == 0 {
		return s.writeJSONResponse(c, &res)
	}

	doc := docs.Get(0).(wirebson.AnyDocument)

	b, err := marshalSingleJSON(doc)
	if err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	res.Document = &b

	return s.writeJSONResponse(c, &res)
}
