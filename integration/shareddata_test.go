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

// This file contains the implementation of `task env-data` command
// that creates collections in the `test` database for manual experiments.
// It is not a real test file but wrapped into one
// because some needed functionality is already available in setup helpers.
// This file is skipped by default; we pass the build tag only in the `task env-data` command.

//go:build docdb_testenvdata

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/hanzoai/docdb/integration/setup"
	"github.com/hanzoai/docdb/integration/shareddata"
)

func TestEnvData(t *testing.T) {
	s := setup.SetupWithOpts(t, nil)

	opts := options.Client().ApplyURI(s.MongoDBURI)
	client, err := mongo.Connect(s.Ctx, opts)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, client.Disconnect(s.Ctx))
	})

	db := client.Database("test")
	require.NoError(t, db.Drop(s.Ctx))

	for _, p := range shareddata.AllProviders() {
		inserted := setup.InsertProviders(t, s.Ctx, db.Collection(p.Name()), p)
		require.True(t, inserted)
	}

	tb := setup.FailsForDocDB(t, "https://github.com/hanzoai/docdb/issues/4303")
	tb = setup.FailsForMongoDB(tb, "https://github.com/hanzoai/docdb/issues/4303")

	inserted := setup.InsertProviders(tb, s.Ctx, db.Collection("All"), shareddata.AllProviders()...)
	require.True(tb, inserted)
}
