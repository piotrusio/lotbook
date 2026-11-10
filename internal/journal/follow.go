package journal

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Entry is a record as it stands in the journal: its partition, its offset
// in that partition, and the record.
type Entry struct {
	Partition int32
	Offset    int64
	Record
}

// Taker is what a follower hands the journal to. It keeps how far each
// partition has been taken, and takes a partition's entries together with
// that, in one step, so that the two never disagree.
type Taker interface {
	// Next is the offset to take next in each of partitions of topic; a
	// partition absent from the result is taken from its start.
	Next(ctx context.Context, topic string, partitions []int32) (map[int32]int64, error)
	// Take takes entries of one partition, in offset order.
	Take(ctx context.Context, topic string, partition int32, entries []Entry) error
}

// Follow reads topic as a member of group, and hands every partition it is
// given to t: from the offset t says, in order, a fetch at a time. Offsets are
// never committed to Kafka; t is where they are kept (ADR-0009). Follow
// returns nil once ctx is done, and the first error of t or of the journal
// otherwise.
func Follow(ctx context.Context, seeds []string, topic, group string, t Taker) error {
	var failed struct {
		sync.Mutex
		err error
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.AdjustFetchOffsetsFn(func(ctx context.Context, offsets map[string]map[int32]kgo.Offset) (map[string]map[int32]kgo.Offset, error) {
			for topic, partitions := range offsets {
				next, err := t.Next(ctx, topic, slices.Collect(maps.Keys(partitions)))
				if err != nil {
					failed.Lock()
					failed.err = fmt.Errorf("journal: where to resume %s: %w", topic, err)
					failed.Unlock()
					return nil, err
				}
				for p := range partitions {
					partitions[p] = kgo.NewOffset().AtStart()
					if o, ok := next[p]; ok {
						partitions[p] = kgo.NewOffset().At(o).WithEpoch(-1)
					}
				}
			}
			return offsets, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	defer client.Close()

	for {
		fetches := client.PollFetches(ctx)
		if ctx.Err() != nil {
			client.AllowRebalance()
			return nil
		}
		failed.Lock()
		err := failed.err
		failed.Unlock()

		fetches.EachError(func(topic string, p int32, e error) {
			if err == nil && !kerr.IsRetriable(e) && !errors.Is(e, context.Canceled) {
				err = fmt.Errorf("journal: fetch %s/%d: %w", topic, p, e)
			}
		})
		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			if err != nil || len(p.Records) == 0 {
				return
			}
			// An entry being taken is finished even when the follower is
			// being stopped; what is not taken is taken by the next one.
			if e := t.Take(context.WithoutCancel(ctx), p.Topic, p.Partition, entries(p.Records)); e != nil {
				err = fmt.Errorf("journal: take %s/%d: %w", p.Topic, p.Partition, e)
			}
		})
		client.AllowRebalance()
		if err != nil {
			return err
		}
	}
}

func entries(records []*kgo.Record) []Entry {
	es := make([]Entry, 0, len(records))
	for _, r := range records {
		e := Entry{Partition: r.Partition, Offset: r.Offset, Record: Record{Source: string(r.Key), Body: r.Value}}
		for _, h := range r.Headers {
			switch h.Key {
			case "lotbook-contract":
				e.Contract = string(h.Value)
			case "traceparent":
				e.TraceParent = string(h.Value)
			}
		}
		es = append(es, e)
	}
	return es
}
