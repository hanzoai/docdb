package zap

import (
	"context"
	"log/slog"

	"github.com/hanzoai/docdb/internal/documentdb"
	zaplib "github.com/luxfi/zap"
)

// Listener manages a ZAP node that accepts binary protocol connections
// and routes them to DocumentDB's internal handler layer.
//
// Architecture:
//
//	ZAP Client (mongo-thinking)
//	  │ ZAP binary (:9654)
//	  ▼
//	DocumentDB Listener (this)
//	  │ translates mongo semantics → SQL
//	  │ via documentdb_api PostgreSQL extension
//	  ▼
//	hanzo/sql (PostgreSQL :9999)
//	  ZAP binary — end-to-end, no JSON, no wire protocol translation
type Listener struct {
	pool    *documentdb.Pool
	node    *zaplib.Node
	handler *Handler
	logger  *slog.Logger
	port    int
}

// NewListener creates a ZAP listener for DocumentDB on port.
//
// Constructing one is already the decision to serve: whether to listen at all
// belongs to the caller reading --listen-zap-addr, and is not asked again here.
func NewListener(pool *documentdb.Pool, logger *slog.Logger, port int) *Listener {
	return &Listener{
		pool:    pool,
		handler: NewHandler(pool, logger),
		logger:  logger.With("component", "zap"),
		port:    port,
	}
}

// Start begins listening for ZAP connections.
func (l *Listener) Start() error {
	l.node = zaplib.NewNode(zaplib.NodeConfig{
		NodeID:      nodeID(),
		Port:        l.port,
		ServiceType: service,
		Logger:      l.logger,
	})

	// Register the DocumentDB message handler.
	// All document operations (find, insert, update, delete, aggregate)
	// come through this single handler and are routed by path.
	l.node.Handle(MsgTypeDocumentDB, func(ctx context.Context, from string, msg *zaplib.Message) (*zaplib.Message, error) {
		return l.handler.HandleMessage(ctx, from, msg)
	})

	if err := l.node.Start(); err != nil {
		return err
	}

	l.logger.Info("ZAP transport listening",
		"port", l.port,
		"discovery", service,
		"nodeID", nodeID(),
	)

	return nil
}

// Stop gracefully shuts down the ZAP listener.
func (l *Listener) Stop() {
	if l.node != nil {
		l.logger.Info("stopping ZAP transport")
		l.node.Stop()
	}
}
