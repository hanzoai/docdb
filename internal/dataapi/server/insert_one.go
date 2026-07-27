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
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/hanzoai/docdb/internal/dataapi/api"
	"github.com/hanzoai/docdb/internal/util/zipapp"
)

// InsertOne implements [ServerInterface].
func (s *Server) InsertOne(c *zip.Ctx) error {
	ctx := c.Context()

	var req api.InsertOneRequestBody
	if err := s.decodeJSONRequest(c, &req); err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	insert, err := unmarshalSingleJSON(&req.Document)
	if err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	insertDoc, ok := insert.(wirebson.AnyDocument)
	if !ok {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.New("document must be a BSON document").Error())
	}

	var doc *wirebson.Document

	doc, err = ensureID(insertDoc)
	if err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	documents := wirebson.MustArray(doc)

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

	insertedId, err := wirebson.ToDriver(doc.Get("_id"))
	if err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	res := api.InsertOneResponseBody{
		InsertedId: &insertedId,
	}

	return s.writeJSONResponse(c, &res)
}

// ensureID ensures that inserted document has an "_id" field.
func ensureID(doc wirebson.AnyDocument) (*wirebson.Document, error) {
	decoded, err := doc.Decode()
	if err != nil {
		return nil, lazyerrors.Error(err)
	}

	for field := range decoded.Fields() {
		if field == "_id" {
			return decoded, nil
		}
	}

	id, err := wirebson.FromDriver(bson.NewObjectID())
	if err != nil {
		return nil, lazyerrors.Error(err)
	}

	err = decoded.Add("_id", id)
	if err != nil {
		return nil, lazyerrors.Error(err)
	}

	return decoded, err
}
