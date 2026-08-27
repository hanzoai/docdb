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

package docdb_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/hanzoai/docdb/docdb"
	"github.com/hanzoai/docdb/internal/util/testutil"
)

func TestDeps(t *testing.T) {
	t.Parallel()

	var res struct {
		Deps []string `json:"Deps"`
	}
	b, err := exec.Command("go", "list", "-json").Output()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &res))

	assert.NotContains(t, res.Deps, "testing", `package "testing" should not be imported by non-testing code`)
}

func Example() {
	f, err := docdb.New(&docdb.Config{
		PostgreSQLURL: "postgres://username:password@127.0.0.1:5432/postgres",
		ListenAddr:    "127.0.0.1:17027",
		StateDir:      ".",
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		f.Run(ctx)
		close(done)
	}()

	uri := f.MongoDBURI()
	fmt.Println(uri)

	// Use MongoDB URI as usual.
	// For example:
	//
	// import "go.mongodb.org/mongo-driver/v2/mongo"
	// import "go.mongodb.org/mongo-driver/v2/mongo/options"
	//
	// [...]
	//
	// mongo.Connect(options.Client().ApplyURI(uri))

	cancel()
	<-done

	// Output: mongodb://127.0.0.1:17027/
}

// start runs an embedded DocDB against the test PostgreSQL and connects the
// real MongoDB driver to it over the wire protocol, which is what makes these
// tests say something about the product: nothing here reaches past the socket
// a customer's driver would use. Both the listener and the client are torn
// down when the test ends.
func start(t *testing.T) (context.Context, *mongo.Client) {
	t.Helper()

	f, err := docdb.New(&docdb.Config{
		PostgreSQLURL: testutil.PostgreSQLURL(t),
		ListenAddr:    "127.0.0.1:0",
		StateDir:      t.TempDir(),
		LogLevel:      slog.LevelDebug,
		LogOutput:     t.Output(),
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(testutil.Ctx(t))
	done := make(chan struct{})

	go func() {
		f.Run(ctx)
		close(done)
	}()

	t.Cleanup(func() {
		cancel()
		<-done
	})

	uri := f.MongoDBURI()
	require.NotEmpty(t, uri)

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)

	// Registered after the listener's, so it runs before it: the client says
	// goodbye while there is still something listening to hear it.
	t.Cleanup(func() {
		require.NoError(t, client.Disconnect(context.WithoutCancel(ctx)))
	})

	return ctx, client
}

func TestDocDB(t *testing.T) {
	ctx, client := start(t)

	require.NoError(t, client.Ping(ctx, nil))

	res := client.Database("admin").RunCommand(ctx, bson.D{{Key: "getFreeMonitoringStatus", Value: 1}})

	var actual bson.D
	err := res.Decode(&actual)
	require.NoError(t, err)

	expected := bson.D{
		{Key: "state", Value: "undecided"},
		{Key: "message", Value: "monitoring is undecided"},
		{Key: "ok", Value: 1.0},
	}
	assert.Equal(t, expected, actual)
}

// doc is a row of the smoke collection, round-tripped through BSON so the
// assertions below compare Go values rather than whatever numeric type the
// driver happened to decode into.
type doc struct {
	Name string `bson:"name"`
	Qty  int    `bson:"qty"`
}

// TestCRUD drives one document through insert, read, update and delete over
// the MongoDB wire protocol. It is the smallest run that proves the whole
// path — driver, wire protocol, command handler, SQL translation and the
// DocumentDB extension — carries a write out and reads the same value back.
func TestCRUD(t *testing.T) {
	ctx, client := start(t)

	db := client.Database("smoke")
	t.Cleanup(func() {
		require.NoError(t, db.Drop(context.WithoutCancel(ctx)))
	})

	coll := db.Collection("crud")

	ins, err := coll.InsertMany(ctx, []any{
		doc{Name: "widget", Qty: 3},
		doc{Name: "gasket", Qty: 7},
		doc{Name: "flange", Qty: 11},
	})
	require.NoError(t, err)
	require.Len(t, ins.InsertedIDs, 3)

	n, err := coll.CountDocuments(ctx, bson.D{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)

	var got doc
	err = coll.FindOne(ctx, bson.D{{Key: "name", Value: "gasket"}}).Decode(&got)
	require.NoError(t, err)
	assert.Equal(t, doc{Name: "gasket", Qty: 7}, got)

	// A filter the server has to evaluate, rather than a primary-key lookup.
	cur, err := coll.Find(ctx, bson.D{{Key: "qty", Value: bson.D{{Key: "$gte", Value: 7}}}},
		options.Find().SetSort(bson.D{{Key: "qty", Value: 1}}))
	require.NoError(t, err)

	var found []doc
	require.NoError(t, cur.All(ctx, &found))
	assert.Equal(t, []doc{{Name: "gasket", Qty: 7}, {Name: "flange", Qty: 11}}, found)

	upd, err := coll.UpdateOne(ctx,
		bson.D{{Key: "name", Value: "widget"}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "qty", Value: 42}}}})
	require.NoError(t, err)
	assert.Equal(t, int64(1), upd.MatchedCount)
	assert.Equal(t, int64(1), upd.ModifiedCount)

	err = coll.FindOne(ctx, bson.D{{Key: "name", Value: "widget"}}).Decode(&got)
	require.NoError(t, err)
	assert.Equal(t, doc{Name: "widget", Qty: 42}, got)

	del, err := coll.DeleteOne(ctx, bson.D{{Key: "name", Value: "flange"}})
	require.NoError(t, err)
	assert.Equal(t, int64(1), del.DeletedCount)

	n, err = coll.CountDocuments(ctx, bson.D{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	// The delete removed the right one, not merely one of them.
	err = coll.FindOne(ctx, bson.D{{Key: "name", Value: "flange"}}).Decode(&got)
	assert.ErrorIs(t, err, mongo.ErrNoDocuments)
}
