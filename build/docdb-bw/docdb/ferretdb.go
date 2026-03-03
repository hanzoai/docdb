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

//go:build docdb_bw

package main

/*
#include "postgres.h"
*/
import "C"

import (
	"context"
	"log"
	"log/slog"
	"os"

	"github.com/hanzoai/docdb/build/version"
	"github.com/hanzoai/docdb/docdb"
	"github.com/hanzoai/docdb/internal/util/ctxutil"
)

//export BackgroundWorkerMain
func BackgroundWorkerMain(args C.Datum) {
	log.SetPrefix("docdb: ")
	log.SetFlags(0)

	ctx, stop := ctxutil.SigTerm(context.Background())
	defer stop()

	f, err := docdb.New(&docdb.Config{
		// TODO https://github.com/hanzoai/docdb/issues/4771
		PostgreSQLURL: "postgres://username:password@127.0.0.1:5432/postgres",
		ListenAddr:    "127.0.0.1:27017",
		StateDir:      ".",
		LogLevel:      slog.LevelDebug,
		LogOutput:     os.Stderr,
	})
	if err != nil {
		log.Fatal(err)
	}

	version.Get().Package = "extension"

	done := make(chan struct{})

	// Use WaitLatch, ResetLatch, and WL_POSTMASTER_DEATH to check if `postmaster` process died and call stop().
	// TODO https://github.com/hanzoai/docdb/issues/4771

	go func() {
		f.Run(ctx)
		close(done)
	}()

	uri := f.MongoDBURI()
	log.Printf("Running at %s", uri)

	<-done

	return
}

func main() {
	panic("not reached")
}
