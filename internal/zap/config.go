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
package zap

import (
	"fmt"
	"os"
)

// Config for the ZAP transport.
type Config struct {
	// Port to listen on (default 9654).
	Port int

	// ServiceType for mDNS discovery (default "_hanzo-documentdb._tcp").
	ServiceType string

	// NodeID for ZAP peer identification.
	NodeID string

	// Enabled controls whether the ZAP listener starts.
	Enabled bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	nodeID, _ := os.Hostname()
	if nodeID == "" {
		nodeID = "documentdb-node"
	}

	port := 9654
	if p := os.Getenv("ZAP_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}

	return Config{
		Port:        port,
		ServiceType: "_hanzo-documentdb._tcp",
		NodeID:      nodeID,
		Enabled:     os.Getenv("ZAP_DISABLED") != "true",
	}
}
