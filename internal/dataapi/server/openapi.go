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
	"github.com/zap-proto/zip"

	"github.com/hanzoai/docdb/internal/dataapi/api"
	"github.com/hanzoai/docdb/internal/util/logging"
)

// OpenAPISpec serves the OpenAPI specification.
func (s *Server) OpenAPISpec(c *zip.Ctx) error {
	c.SetHeader("Content-Type", "application/json")

	if err := c.Fiber().Send(api.Spec); err != nil {
		s.l.WarnContext(c.Context(), "Failed to write OpenAPI spec", logging.Error(err))
	}

	return nil
}
