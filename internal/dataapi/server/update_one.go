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
	"github.com/AlekSi/pointer"
	"github.com/FerretDB/wire/wirebson"
	"github.com/zap-proto/zip"

	"github.com/hanzoai/docdb/internal/dataapi/api"
	"github.com/hanzoai/docdb/internal/util/must"
	"github.com/hanzoai/docdb/internal/util/zipapp"
)

// UpdateOne implements [ServerInterface].
func (s *Server) UpdateOne(c *zip.Ctx) error {
	ctx := c.Context()

	var req api.UpdateRequestBody
	if err := s.decodeJSONRequest(c, &req); err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	updateDoc, err := prepareDocument(
		"q", req.Filter,
		"u", req.Update,
		"upsert", req.Upsert,
		"multi", false,
	)
	if err != nil {
		return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
	}

	msg, err := prepareRequest(
		"update", req.Collection,
		"$db", req.Database,
		"updates", wirebson.MustArray(updateDoc),
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

	res := api.UpdateResponseBody{
		MatchedCount:  resp.Document().Get("n").(int32),
		ModifiedCount: resp.Document().Get("nModified").(int32),
	}

	if upsertedRaw := resp.Document().Get("upserted"); upsertedRaw != nil {
		upserted := must.NotFail(upsertedRaw.(wirebson.AnyArray).Decode())

		if upserted.Len() > 0 {
			item := must.NotFail(upserted.Get(0).(wirebson.AnyDocument).Decode())

			var upsertedId any

			upsertedId, err = wirebson.ToDriver(item.Get("_id"))
			if err != nil {
				return zipapp.Text(c, http.StatusInternalServerError, lazyerrors.Error(err).Error())
			}

			res.UpsertedId = pointer.To(fmt.Sprint(upsertedId))
		}
	}

	return s.writeJSONResponse(c, &res)
}
