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

//go:build ignore

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/hanzoai/docdb/internal/util/must"
)

// runGit runs `git` with given arguments and returns stdout.
func runGit(args ...string) []byte {
	cmd := exec.Command("git", args...)
	cmd.Stderr = os.Stderr

	b, err := cmd.Output()
	if err != nil {
		panic(fmt.Sprintf("Failed to run %q: %s", strings.Join(cmd.Args, " "), err))
	}

	return b
}

// saveFile stores the given bytes in the given file with logging.
func saveFile(b []byte, filename string) {
	log.Printf("%s: %s", filename, b)
	must.NoError(os.WriteFile(filename, b, 0o666))
}

func main() {
	log.SetFlags(0)

	var wg sync.WaitGroup

	// git describe --dirty > version.txt
	//
	// The v1.24.x tags are excluded because they are not releases of this
	// product — CHANGELOG.md says so in as many words. They sit on commits
	// newer than v2.8.2, so a plain describe answers the nearest one and every
	// binary built since has called itself v1.24.5: a version that names the v1
	// line, which is a different backend from the one this tree compiles. The
	// string is not cosmetic — it is what buildInfo and serverStatus hand a
	// connected client, so a driver gating on it gates on the wrong number.
	//
	// Excluding rather than matching "v2.*" keeps the next major working
	// without a second edit here.
	wg.Add(1)
	go func() {
		defer wg.Done()

		saveFile(runGit("describe", "--dirty", "--exclude", "v1.24.*"), "version.txt")
	}()

	// git rev-parse HEAD > commit.txt
	wg.Add(1)
	go func() {
		defer wg.Done()

		saveFile(runGit("rev-parse", "HEAD"), "commit.txt")
	}()

	// git branch --show-current > branch.txt
	wg.Add(1)
	go func() {
		defer wg.Done()

		saveFile(runGit("branch", "--show-current"), "branch.txt")
	}()

	// output package.txt in the same format just for logging
	b, _ := os.ReadFile("package.txt")
	log.Printf("package.txt: %s", b)

	wg.Wait()
}
