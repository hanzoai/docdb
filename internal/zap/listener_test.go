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

package zap

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accepting reports whether something answers a TCP connection on port.
func accepting(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}

	_ = conn.Close()

	return true
}

// freePort returns a port with nothing on it, by taking one and giving it back.
func freePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())

	return port
}

// TestListenerServesOnlyWhenStarted pins the half of the default that this
// package owns: the port is open because Start was called, and for no other
// reason. Nothing here consults the environment, so no ZAP_PORT or ZAP_DISABLED
// can move it — the caller's address is the only input.
func TestListenerServesOnlyWhenStarted(t *testing.T) {
	port := freePort(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	require.False(t, accepting(port), "port is already in use; the test cannot say anything")

	// A pool is what the handler queries once a message arrives; listening
	// never touches it, which is why this needs no database.
	l := NewListener(nil, logger, port)

	require.False(t, accepting(port), "constructing a listener opened the port")

	require.NoError(t, l.Start())
	t.Cleanup(l.Stop)

	assert.Eventually(t, func() bool { return accepting(port) }, 5*time.Second, 20*time.Millisecond,
		"Start returned but nothing accepts on the port")

	l.Stop()

	assert.Eventually(t, func() bool { return !accepting(port) }, 5*time.Second, 20*time.Millisecond,
		"Stop returned but something still accepts on the port")
}

func TestPort(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want int
	}{
		{addr: ":9654", want: 9654},
		{addr: ":0", want: 0},
	} {
		t.Run(tc.addr, func(t *testing.T) {
			got, err := Port(tc.addr)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	// A host is refused rather than dropped. luxfi/zap listens on every
	// interface, so honoring one of these would mean saying so and doing
	// otherwise.
	for _, addr := range []string{
		"127.0.0.1:9654",
		"localhost:9654",
		"0.0.0.0:9654",
		"[::1]:9654",
	} {
		t.Run(addr, func(t *testing.T) {
			_, err := Port(addr)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not honored")
		})
	}

	for _, addr := range []string{"9654", "", ":http", ":-1"} {
		t.Run("invalid/"+addr, func(t *testing.T) {
			_, err := Port(addr)
			assert.Error(t, err)
		})
	}
}
