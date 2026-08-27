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

package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hanzoai/docdb/internal/util/testutil"
)

func TestVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode")
	}

	t.Parallel()

	ctx, cancel := context.WithTimeout(testutil.Ctx(t), 5*time.Second)
	t.Cleanup(cancel)

	bin := filepath.Join(testutil.BinDir, "docdb")

	cmd := exec.CommandContext(ctx, bin, "--version")
	b, err := cmd.Output()
	require.NoError(t, err)
	assert.Regexp(t, `version: v([0-9]+)\.([0-9]+)\.([0-9]+)`, string(b))
	assert.Regexp(t, `branch: \w+`, string(b))
	commit := regexp.MustCompile(`commit: ([0-9a-f]{40})`).FindStringSubmatch(string(b))
	require.Len(t, commit, 2)

	cmd = exec.CommandContext(ctx, "go", "version", "-m", bin)
	b, err = cmd.Output()
	require.NoError(t, err)
	revision := regexp.MustCompile(`vcs.revision=([0-9a-f]{40})`).FindStringSubmatch(string(b))
	require.NotEmpty(t, revision)
	require.Len(t, revision, 2)

	assert.Equal(t, commit[1], revision[1])
}

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

// TestZAPListenerIsOffByDefault pins the default that keeps an unauthenticated,
// plaintext door onto the whole dataset shut. The ZAP listener reaches the same
// DocumentDB pool the MongoDB port does but asks nothing of the caller, so a
// default that starts it hands the data to whoever can route to the port.
//
// Reaching it must be a decision someone wrote down, which is what "off unless
// an address is given" means. [checkFlags] turns the "-" into the empty string
// that [setup.Setup] reads as "do not listen".
func TestZAPListenerIsOffByDefault(t *testing.T) {
	// DefaultEnvars("DOCDB") would otherwise let the ambient environment
	// answer the question this test is asking.
	if v, ok := os.LookupEnv("DOCDB_LISTEN_ZAP_ADDR"); ok {
		require.NoError(t, os.Unsetenv("DOCDB_LISTEN_ZAP_ADDR"))
		t.Cleanup(func() { require.NoError(t, os.Setenv("DOCDB_LISTEN_ZAP_ADDR", v)) })
	}

	parser, err := kong.New(&cli, kongOptions...)
	require.NoError(t, err)

	_, err = parser.Parse(nil)
	require.NoError(t, err)

	assert.Equal(t, "-", cli.Listen.ZAPAddr, "the ZAP listener must be off unless asked for")

	checkFlags(slog.New(slog.NewTextHandler(io.Discard, nil)))

	assert.Empty(t, cli.Listen.ZAPAddr, "checkFlags must leave setup nothing to listen on")
}
