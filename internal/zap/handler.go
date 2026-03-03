package zap

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/hanzoai/docdb/internal/documentdb"
	zaplib "github.com/luxfi/zap"
)

// ZAP message type for DocumentDB operations.
const MsgTypeDocumentDB uint16 = 303

// ZAP field offsets (matching hanzo/orm protocol).
const (
	fieldPath  = 4  // Text: operation path
	fieldBody  = 12 // Bytes: JSON body
	respStatus = 0  // Uint32: status code
	respBody   = 4  // Bytes: response JSON
)

// Handler translates ZAP messages into DocumentDB operations.
// It bridges "mongo-thinking" ZAP clients to PostgreSQL storage.
type Handler struct {
	pool   *documentdb.Pool
	logger *slog.Logger
}

// NewHandler creates a ZAP message handler backed by a DocumentDB pool.
func NewHandler(pool *documentdb.Pool, logger *slog.Logger) *Handler {
	return &Handler{pool: pool, logger: logger}
}

// HandleMessage is the main ZAP message handler for DocumentDB operations.
// It dispatches based on the path field in the ZAP message:
//   - /find      → MongoDB-style find (filter, projection, sort, limit)
//   - /insert    → Insert documents
//   - /update    → Update documents (filter + update operators)
//   - /delete    → Delete documents (filter)
//   - /aggregate → Aggregation pipeline
//   - /count     → Count documents matching filter
func (h *Handler) HandleMessage(ctx context.Context, from string, msg *zaplib.Message) (*zaplib.Message, error) {
	root := msg.Root()
	path := root.Text(fieldPath)
	body := root.Bytes(fieldBody)

	h.logger.Debug("zap: documentdb", "path", path, "from", from, "bodyLen", len(body))

	var req map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return errorResponse(400, "invalid JSON body")
		}
	}

	switch path {
	case "/find":
		return h.handleFind(ctx, req)
	case "/insert":
		return h.handleInsert(ctx, req)
	case "/update":
		return h.handleUpdate(ctx, req)
	case "/delete":
		return h.handleDelete(ctx, req)
	case "/aggregate":
		return h.handleAggregate(ctx, req)
	case "/count":
		return h.handleCount(ctx, req)
	default:
		return errorResponse(400, fmt.Sprintf("unknown path: %s", path))
	}
}

// handleFind translates a ZAP find request to DocumentDB/PostgreSQL.
// Request: { "database": "db", "collection": "col", "filter": {...}, "limit": N }
func (h *Handler) handleFind(ctx context.Context, req map[string]interface{}) (*zaplib.Message, error) {
	db, col, err := extractDBCol(req)
	if err != nil {
		return errorResponse(400, err.Error())
	}

	filter, _ := json.Marshal(req["filter"])
	limit := 0
	if l, ok := req["limit"].(float64); ok {
		limit = int(l)
	}

	var documents []json.RawMessage

	err = h.pool.WithConn(func(conn *pgx.Conn) error {
		query := fmt.Sprintf(
			`SELECT documentdb_api.find('%s', '%s', '%s')`,
			db, col, string(filter),
		)
		if limit > 0 {
			query = fmt.Sprintf(
				`SELECT documentdb_api.find('%s', '%s', '{"find": "%s", "filter": %s, "limit": %d}')`,
				db, col, col, string(filter), limit,
			)
		}

		rows, qErr := conn.Query(ctx, query)
		if qErr != nil {
			return qErr
		}
		defer rows.Close()

		for rows.Next() {
			var doc []byte
			if err := rows.Scan(&doc); err != nil {
				continue
			}
			documents = append(documents, doc)
		}
		return rows.Err()
	})
	if err != nil {
		return errorResponse(500, "query error: "+err.Error())
	}

	return jsonResponse(200, map[string]interface{}{
		"documents": documents,
		"count":     len(documents),
	})
}

// handleInsert translates a ZAP insert request.
// Request: { "database": "db", "collection": "col", "documents": [...] }
func (h *Handler) handleInsert(ctx context.Context, req map[string]interface{}) (*zaplib.Message, error) {
	db, col, err := extractDBCol(req)
	if err != nil {
		return errorResponse(400, err.Error())
	}

	docs, ok := req["documents"].([]interface{})
	if !ok {
		return errorResponse(400, "documents array required")
	}

	docsJSON, _ := json.Marshal(docs)

	err = h.pool.WithConn(func(conn *pgx.Conn) error {
		query := fmt.Sprintf(
			`SELECT documentdb_api.insert('%s', '%s', '%s')`,
			db, col, string(docsJSON),
		)
		_, execErr := conn.Exec(ctx, query)
		return execErr
	})
	if err != nil {
		return errorResponse(500, "insert error: "+err.Error())
	}

	return jsonResponse(200, map[string]interface{}{
		"inserted": len(docs),
	})
}

// handleUpdate translates a ZAP update request.
// Request: { "database": "db", "collection": "col", "filter": {...}, "update": {...} }
func (h *Handler) handleUpdate(ctx context.Context, req map[string]interface{}) (*zaplib.Message, error) {
	db, col, err := extractDBCol(req)
	if err != nil {
		return errorResponse(400, err.Error())
	}

	filter, _ := json.Marshal(req["filter"])
	update, _ := json.Marshal(req["update"])

	err = h.pool.WithConn(func(conn *pgx.Conn) error {
		query := fmt.Sprintf(
			`SELECT documentdb_api.update('%s', '%s', '{"q": %s, "u": %s}')`,
			db, col, string(filter), string(update),
		)
		_, execErr := conn.Exec(ctx, query)
		return execErr
	})
	if err != nil {
		return errorResponse(500, "update error: "+err.Error())
	}

	return jsonResponse(200, map[string]string{"status": "ok"})
}

// handleDelete translates a ZAP delete request.
// Request: { "database": "db", "collection": "col", "filter": {...} }
func (h *Handler) handleDelete(ctx context.Context, req map[string]interface{}) (*zaplib.Message, error) {
	db, col, err := extractDBCol(req)
	if err != nil {
		return errorResponse(400, err.Error())
	}

	filter, _ := json.Marshal(req["filter"])

	err = h.pool.WithConn(func(conn *pgx.Conn) error {
		query := fmt.Sprintf(
			`SELECT documentdb_api.delete('%s', '%s', '{"q": %s}')`,
			db, col, string(filter),
		)
		_, execErr := conn.Exec(ctx, query)
		return execErr
	})
	if err != nil {
		return errorResponse(500, "delete error: "+err.Error())
	}

	return jsonResponse(200, map[string]string{"status": "ok"})
}

// handleAggregate translates a ZAP aggregation pipeline request.
// Request: { "database": "db", "collection": "col", "pipeline": [...] }
func (h *Handler) handleAggregate(ctx context.Context, req map[string]interface{}) (*zaplib.Message, error) {
	db, col, err := extractDBCol(req)
	if err != nil {
		return errorResponse(400, err.Error())
	}

	pipeline, _ := json.Marshal(req["pipeline"])

	var results []json.RawMessage

	err = h.pool.WithConn(func(conn *pgx.Conn) error {
		query := fmt.Sprintf(
			`SELECT documentdb_api.aggregate('%s', '{"aggregate": "%s", "pipeline": %s, "cursor": {}}')`,
			db, col, string(pipeline),
		)

		rows, qErr := conn.Query(ctx, query)
		if qErr != nil {
			return qErr
		}
		defer rows.Close()

		for rows.Next() {
			var doc []byte
			if err := rows.Scan(&doc); err != nil {
				continue
			}
			results = append(results, doc)
		}
		return rows.Err()
	})
	if err != nil {
		return errorResponse(500, "aggregate error: "+err.Error())
	}

	return jsonResponse(200, map[string]interface{}{
		"results": results,
	})
}

// handleCount translates a ZAP count request.
func (h *Handler) handleCount(ctx context.Context, req map[string]interface{}) (*zaplib.Message, error) {
	db, col, err := extractDBCol(req)
	if err != nil {
		return errorResponse(400, err.Error())
	}

	filter, _ := json.Marshal(req["filter"])

	var count int

	err = h.pool.WithConn(func(conn *pgx.Conn) error {
		query := fmt.Sprintf(
			`SELECT documentdb_api.count('%s', '{"count": "%s", "query": %s}')`,
			db, col, string(filter),
		)
		return conn.QueryRow(ctx, query).Scan(&count)
	})
	if err != nil {
		return errorResponse(500, "count error: "+err.Error())
	}

	return jsonResponse(200, map[string]interface{}{"count": count})
}

// --- Helpers ---

func extractDBCol(req map[string]interface{}) (string, string, error) {
	db, _ := req["database"].(string)
	col, _ := req["collection"].(string)
	if db == "" {
		return "", "", fmt.Errorf("database required")
	}
	if col == "" {
		return "", "", fmt.Errorf("collection required")
	}
	return db, col, nil
}

func jsonResponse(status uint32, data interface{}) (*zaplib.Message, error) {
	body, _ := json.Marshal(data)
	return buildResponse(status, body)
}

func errorResponse(status uint32, message string) (*zaplib.Message, error) {
	body, _ := json.Marshal(map[string]string{"error": message})
	return buildResponse(status, body)
}

func buildResponse(status uint32, body []byte) (*zaplib.Message, error) {
	b := zaplib.NewBuilder(len(body) + 128)
	obj := b.StartObject(20)
	obj.SetUint32(respStatus, status)
	obj.SetBytes(respBody, body)
	obj.FinishAsRoot()
	data := b.Finish()

	msg, err := zaplib.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("zap: build response: %w", err)
	}
	return msg, nil
}
