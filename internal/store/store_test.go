package store_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lotbook.org/lotbook/internal/journal"
	"lotbook.org/lotbook/internal/store"
)

func TestMigrate_createsTheSchemaAndAgainChangesNothing(t *testing.T) {
	pool := freshSchema(t)

	if err := store.Check(t.Context(), pool); err == nil {
		t.Fatal("Check passed on an empty database")
	}
	for range 2 {
		if err := store.Migrate(t.Context(), pool); err != nil {
			t.Fatalf("Migrate: %v", err)
		}
		if err := store.Check(t.Context(), pool); err != nil {
			t.Fatalf("Check after Migrate: %v", err)
		}
	}
	for _, table := range []string{"journal_offsets", "skeleton_records"} {
		var exists bool
		if err := pool.QueryRow(t.Context(), `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Errorf("table %s exists = %v, %v; want it created", table, exists, err)
		}
	}
}

func TestTake_keepsTheRecordsAndTheNextOffsetTogether(t *testing.T) {
	s, pool := migrated(t)

	if err := s.Take(t.Context(), "journal", 1, entries(1, 0, 3)); err != nil {
		t.Fatalf("Take: %v", err)
	}

	next, err := s.Next(t.Context(), "journal", []int32{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 || next[1] != 3 {
		t.Errorf("Next = %v; want partition 1 at 3 and partition 2 absent", next)
	}
	got := taken(t, pool, 1)
	if len(got) != 3 {
		t.Fatalf("got %d records; want 3", len(got))
	}
	for i, r := range got {
		want := entries(1, 0, 3)[i]
		if r.Offset != want.Offset || r.Source != want.Source || string(r.Body) != string(want.Body) ||
			r.Contract != want.Contract || r.TraceParent != want.TraceParent {
			t.Errorf("record %d = %+v; want %+v", i, r, want)
		}
	}
}

func TestTake_dropsRecordsAlreadyTaken(t *testing.T) {
	s, pool := migrated(t)

	if err := s.Take(t.Context(), "journal", 0, entries(0, 0, 3)); err != nil {
		t.Fatal(err)
	}
	if err := s.Take(t.Context(), "journal", 0, entries(0, 1, 5)); err != nil {
		t.Fatalf("Take with records already taken: %v", err)
	}

	var offsets []int64
	for _, r := range taken(t, pool, 0) {
		offsets = append(offsets, r.Offset)
	}
	if len(offsets) != 5 || offsets[0] != 0 || offsets[4] != 4 {
		t.Errorf("offsets taken = %v; want 0 to 4, each once", offsets)
	}
	if next, _ := s.Next(t.Context(), "journal", []int32{0}); next[0] != 5 {
		t.Errorf("next = %d; want 5", next[0])
	}
}

func TestTake_refusesAGapAndWritesNothing(t *testing.T) {
	s, pool := migrated(t)
	if err := s.Take(t.Context(), "journal", 0, entries(0, 0, 3)); err != nil {
		t.Fatal(err)
	}

	if err := s.Take(t.Context(), "journal", 0, entries(0, 5, 2)); err == nil {
		t.Fatal("Take accepted records after a gap; offsets 3 and 4 would be skipped")
	}

	if got := len(taken(t, pool, 0)); got != 3 {
		t.Errorf("got %d records after a refused Take; want 3", got)
	}
	if next, _ := s.Next(t.Context(), "journal", []int32{0}); next[0] != 3 {
		t.Errorf("next = %d; want 3", next[0])
	}
}

func entries(partition int32, from int64, n int) []journal.Entry {
	es := make([]journal.Entry, 0, n)
	for o := from; o < from+int64(n); o++ {
		es = append(es, journal.Entry{
			Partition: partition,
			Offset:    o,
			Record: journal.Record{
				Source:      "plant-a.erp",
				Body:        []byte(`{"source": "plant-a.erp", "position": "` + strconv.FormatInt(o, 10) + `"}`),
				Contract:    "ingest/v1",
				TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			},
		})
	}
	return es
}

func taken(t *testing.T, pool *pgxpool.Pool, partition int32) []journal.Entry {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		SELECT journal_offset, source, body, contract, traceparent
		FROM skeleton_records WHERE topic = 'journal' AND partition = $1
		ORDER BY journal_offset`, partition)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (journal.Entry, error) {
		e := journal.Entry{Partition: partition}
		var body string
		err := row.Scan(&e.Offset, &e.Source, &body, &e.Contract, &e.TraceParent)
		e.Body = []byte(body)
		return e, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func migrated(t *testing.T) (*store.Store, *pgxpool.Pool) {
	t.Helper()
	pool := freshSchema(t)
	if err := store.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	return store.New(pool), pool
}

// freshSchema connects to the database named by LOTBOOK_TEST_POSTGRES_URL
// inside a schema of its own, so that every test starts from nothing.
func freshSchema(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("LOTBOOK_TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("LOTBOOK_TEST_POSTGRES_URL is not set; this test needs a PostgreSQL")
	}
	name := "test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	admin, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(t.Context())
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if c, err := pgx.Connect(ctx, url); err == nil {
			_, _ = c.Exec(ctx, "DROP SCHEMA "+name+" CASCADE")
			_ = c.Close(ctx)
		}
	})

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = name
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
