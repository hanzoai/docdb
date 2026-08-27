// Package zap provides a ZAP binary protocol listener for DocumentDB.
//
// This allows clients to communicate using the ZAP zero-copy protocol
// instead of MongoDB wire protocol. ZAP messages are translated to
// the same internal handler operations — find, insert, update, delete,
// aggregate — that MongoDB wire protocol uses.
//
// The key insight: DocumentDB stores data in PostgreSQL (hanzo/sql).
// With native ZAP, the path is ZAP→DocumentDB→ZAP→PostgreSQL.
// Wire format stays binary end-to-end; only the semantic translation
// (MongoDB query language → SQL) happens in between.
//
// # Reaching this listener
//
// --listen-zap-addr is the only thing that starts it, and it is off unless
// given an address. Nothing here authenticates, and the transport is
// plaintext, so the port is the whole of the access control: whoever can
// reach it can read and write every collection in the pool.
//
// The address's host must be empty — see [Port].
package zap

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

// service is the mDNS service type the node advertises itself under.
const service = "_hanzo-documentdb._tcp"

// nodeID identifies this node to ZAP peers.
func nodeID() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}

	return "docdb"
}

// Port returns the TCP port named by a --listen-zap-addr value.
//
// The host must be empty. [github.com/luxfi/zap] binds every interface —
// Node.Start listens on fmt.Sprintf(":%d", port) and NodeConfig has no field
// to say otherwise — so a host here would be an instruction this package
// cannot carry out. Accepting "127.0.0.1:9654" and then listening on every
// address is the one failure worth refusing outright: it reads as a bound
// listener and answers as an open one.
//
// Honoring the host needs a bind address in luxfi/zap's NodeConfig, and
// encrypting the transport needs its TLS field filled; both are upstream work
// in that package, not here.
func Port(addr string) (int, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("--listen-zap-addr=%q: %w", addr, err)
	}

	if host != "" {
		return 0, fmt.Errorf(
			"--listen-zap-addr=%q: the host is not honored, so naming one would promise "+
				"an interface this listener does not bind; write %q to accept every interface",
			addr, ":"+port,
		)
	}

	// ParseUint with 16 bits is the range check: Atoi accepts ":-1", and a
	// negative port reaches net.Listen as a string it cannot explain.
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("--listen-zap-addr=%q: %w", addr, err)
	}

	return int(n), nil
}
