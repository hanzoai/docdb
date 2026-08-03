package zap

import (
	"encoding/json"
	"strings"
	"testing"
)

// Everything the ZAP caller sends — the database name, the collection name, the
// filter, the documents, the pipeline — arrives as JSON in an unauthenticated
// message on a plaintext port. None of it may become SQL text.
//
// These payloads are the ones that matter against PostgreSQL:
//
//   - a single quote closes the literal, so anything after it is parsed as SQL;
//   - encoding/json escapes " and \ but NOT ', so a quote inside a filter value
//     or a document reaches the statement byte for byte;
//   - conn.Exec with no bind arguments uses pgx's simple protocol, which sends
//     the text verbatim and lets PostgreSQL run every statement in it, so a
//     payload that keeps the first statement valid gets the rest executed too.
//
// The assertions are on the SQL the handler hands to pgx, so they fail if the
// construction ever goes back to interpolation.
const (
	stackPayload = `x', '{"insert":"c","documents":[]}'); select pg_read_file('/etc/passwd'); --`
	readPayload  = `x') || (select current_setting('data_directory')) || ('`
)

// marker fragments that must never appear in the SQL text
var leaks = []string{"pg_read_file", "current_setting", "select ", "--", "'"}

func assertNoLeak(t *testing.T, what, sql string, args []any, payload string) {
	t.Helper()

	for _, frag := range leaks {
		if strings.Contains(payload, frag) && strings.Contains(sql, frag) {
			t.Errorf("%s: payload fragment %q reached the SQL text: %s", what, frag, sql)
		}
	}

	var found bool
	for _, a := range args {
		if s, ok := a.(string); ok && strings.Contains(s, payload) {
			found = true
		}
	}
	if !found {
		t.Errorf("%s: payload never appeared in the bind arguments (%v); SQL = %s", what, args, sql)
	}
}

func TestQueries_DatabaseAndCollectionAreBound(t *testing.T) {
	empty := json.RawMessage(`{}`)

	for _, payload := range []string{stackPayload, readPayload} {
		for _, tc := range []struct {
			name string
			run  func(v string) (string, []any)
		}{
			{"find", func(v string) (string, []any) { return findQuery(v, v, empty, 0) }},
			{"find+limit", func(v string) (string, []any) { return findQuery(v, v, empty, 10) }},
			{"insert", func(v string) (string, []any) { return insertQuery(v, v, json.RawMessage(`[]`)) }},
			{"update", func(v string) (string, []any) { return updateQuery(v, v, empty, empty) }},
			{"delete", func(v string) (string, []any) { return deleteQuery(v, v, empty) }},
			{"aggregate", func(v string) (string, []any) { return aggregateQuery(v, v, json.RawMessage(`[]`)) }},
			{"count", func(v string) (string, []any) { return countQuery(v, v, empty) }},
		} {
			sql, args := tc.run(payload)
			assertNoLeak(t, tc.name, sql, args, payload)
		}
	}
}

// The filter, the update, the pipeline and the documents are caller JSON. A
// single quote inside any of them is not escaped by encoding/json, so they have
// to be bound too, not spliced into a quoted literal.
func TestQueries_CallerJSONIsBound(t *testing.T) {
	// json.Marshal of this map keeps the quote intact, which is the point.
	dirty, err := json.Marshal(map[string]any{"a": readPayload})
	if err != nil {
		t.Fatal(err)
	}

	for name, sql := range map[string]string{
		"find":       first(findQuery("db", "col", dirty, 0)),
		"find+limit": first(findQuery("db", "col", dirty, 10)),
		"insert":     first(insertQuery("db", "col", dirty)),
		"update":     first(updateQuery("db", "col", dirty, dirty)),
		"delete":     first(deleteQuery("db", "col", dirty)),
		"aggregate":  first(aggregateQuery("db", "col", dirty)),
		"count":      first(countQuery("db", "col", dirty)),
	} {
		if strings.Contains(sql, "current_setting") || strings.Contains(sql, "'") {
			t.Errorf("%s: caller JSON reached the SQL text: %s", name, sql)
		}
	}
}

// The statement pgx receives must be a constant with only placeholders in it,
// which is also what stops the simple protocol from running a second statement.
func TestQueries_SQLIsConstant(t *testing.T) {
	empty := json.RawMessage(`{}`)

	for name, got := range map[string]string{
		"find":       first(findQuery("db", "col", empty, 0)),
		"find+limit": first(findQuery("db", "col", empty, 10)),
		"insert":     first(insertQuery("db", "col", empty)),
		"update":     first(updateQuery("db", "col", empty, empty)),
		"delete":     first(deleteQuery("db", "col", empty)),
		"aggregate":  first(aggregateQuery("db", "col", empty)),
		"count":      first(countQuery("db", "col", empty)),
	} {
		if strings.ContainsAny(got, "'\"") {
			t.Errorf("%s: SQL still carries a literal, so something is being interpolated: %s", name, got)
		}
		if !strings.Contains(got, "$1") {
			t.Errorf("%s: SQL has no bind placeholder: %s", name, got)
		}
	}
}

func first(sql string, _ []any) string { return sql }
