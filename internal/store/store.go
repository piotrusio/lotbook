package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lotbook.org/lotbook/internal/journal"
)

// Store keeps what the workers take from the journal together with how far
// they have taken it, so that a worker that fails, or a partition handed to
// another worker, resumes exactly where the last transaction ended.
type Store struct {
	pool *pgxpool.Pool
}

// New makes a Store on pool, whose schema Migrate has brought up to date.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Next is the offset to take next in each of topic's partitions; a partition
// nothing has been taken from yet is absent.
func (s *Store) Next(ctx context.Context, topic string, partitions []int32) (map[int32]int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT partition, next_offset FROM journal_offsets
		WHERE topic = $1 AND partition = ANY($2)`, topic, partitions)
	if err != nil {
		return nil, fmt.Errorf("store: next offsets: %w", err)
	}
	next := map[int32]int64{}
	var p int32
	var o int64
	_, err = pgx.ForEachRow(rows, []any{&p, &o}, func() error {
		next[p] = o
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("store: next offsets: %w", err)
	}
	return next, nil
}

// Take keeps entries, all of one partition and in offset order, and moves the
// partition's next offset past the last of them, in one transaction. Entries
// below the next offset were taken before and are dropped; an entry above it
// would leave a gap, and Take refuses it and writes nothing.
func (s *Store) Take(ctx context.Context, topic string, partition int32, entries []journal.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO journal_offsets (topic, partition, next_offset) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, topic, partition, entries[0].Offset); err != nil {
			return err
		}
		var next int64
		if err := tx.QueryRow(ctx, `
			SELECT next_offset FROM journal_offsets
			WHERE topic = $1 AND partition = $2 FOR UPDATE`, topic, partition).Scan(&next); err != nil {
			return err
		}

		fresh := entries
		for len(fresh) > 0 && fresh[0].Offset < next {
			fresh = fresh[1:]
		}
		if len(fresh) == 0 {
			return nil
		}
		if fresh[0].Offset != next {
			return fmt.Errorf("%w: partition %d is taken up to offset %d and the next entry is at %d",
				errGap, partition, next, fresh[0].Offset)
		}

		rows := make([][]any, 0, len(fresh))
		for _, e := range fresh {
			rows = append(rows, []any{topic, partition, e.Offset, e.Source, string(e.Body), e.Contract, e.TraceParent})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"skeleton_records"},
			[]string{"topic", "partition", "journal_offset", "source", "body", "contract", "traceparent"},
			pgx.CopyFromRows(rows)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE journal_offsets SET next_offset = $3
			WHERE topic = $1 AND partition = $2`, topic, partition, fresh[len(fresh)-1].Offset+1)
		return err
	})
	if err != nil {
		return fmt.Errorf("store: take %s/%d: %w", topic, partition, err)
	}
	return nil
}

var errGap = errors.New("a record would be skipped")
