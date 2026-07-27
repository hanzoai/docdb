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
	"fmt"
	"net/http"

	"github.com/AlekSi/lazyerrors"
	"github.com/FerretDB/wire/wirebson"
	"github.com/zap-proto/zip"

	"github.com/hanzoai/docdb/internal/dataapi/api"
	"github.com/hanzoai/docdb/internal/util/zipapp"
)

// InsertMany implements [ServerInterface].
func (s *Server) InsertMany(c *zip.Ctx) error {
	ctx := c.Context()

	var req api.InsertManyRequestBody
	if err := s.decodeJSONRequest(c, &req); err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	docsArr, err := unmarshalSingleJSON(&req.Documents)
	if err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	documents, err := docsArr.(wirebson.RawArray).Decode()
	if err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	var insertedIds []any

	for i, v := range documents.All() {
		v, ok := v.(wirebson.AnyDocument)
		if !ok {
			return zipapp.Text(c, http.StatusBadRequest, fmt.Sprintf("document %d is not a valid BSON document", i))
		}

		var doc *wirebson.Document

		doc, err = ensureID(v)
		if err != nil {
			return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
		}

		var insertedId any

		insertedId, err = wirebson.ToDriver(doc.Get("_id"))
		if err != nil {
			return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
		}

		if err = documents.Replace(i, doc); err != nil {
			return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
		}

		insertedIds = append(insertedIds, insertedId)
	}

	msg, err := prepareRequest(
		"insert", req.Collection,
		"$db", req.Database,
		"documents", documents,
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

	res := api.InsertManyResponseBody{
		InsertedIds: &insertedIds,
	}

	return s.writeJSONResponse(c, &res)
}
