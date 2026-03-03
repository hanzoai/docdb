// Copyright 2021 DocDB Inc.
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

package integration

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/hanzoai/docdb/integration/shareddata"
)

func TestAggregateCompatMatchExpr(t *testing.T) {
	t.Parallel()

	testCases := map[string]aggregateStagesCompatTestCase{
		"Expression": {
			pipeline: bson.A{
				bson.D{{"$match", bson.D{{"$expr", "$v"}}}},
			},
			failsForDocDB: "https://github.com/hanzoai/docdb-DocumentDB/issues/362",
			failsProviders:   []shareddata.Provider{shareddata.Decimal128s, shareddata.Doubles, shareddata.Int64s, shareddata.Scalars},
		},
		"Sum": {
			pipeline: bson.A{bson.D{{"$match", bson.D{
				{"$expr", bson.D{{"$sum", "$v"}}},
			}}}},
			failsForDocDB: "https://github.com/hanzoai/docdb-DocumentDB/issues/362",
			failsProviders:   []shareddata.Provider{shareddata.Decimal128s, shareddata.Doubles, shareddata.Int64s, shareddata.Scalars},
		},
		"Gt": {
			pipeline: bson.A{bson.D{{"$match", bson.D{
				{"$expr", bson.D{{"$gt", bson.A{"$v", 2}}}},
			}}}},
			skip: "https://github.com/hanzoai/docdb/issues/1456",
		},
	}

	testAggregateStagesCompat(t, testCases)
}
