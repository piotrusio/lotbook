package journal_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"lotbook.org/lotbook/internal/journal"
)

func TestFollow_takesEveryRecordFromWhereTheTakerLeftOff(t *testing.T) {
	seeds := brokers(t)
	topic := createTopic(t, seeds, 3)
	appendRecords(t, seeds, topic, 0, 30)
	ends := partitionEnds(t, seeds, topic, 30)

	taker := newFakeTaker()
	var resumed int32 = -1
	for p, end := range ends {
		if end >= 2 {
			resumed = p
			break
		}
	}
	taker.next[resumed] = 2 // taken up to offset 2 before this follower started

	stop := follow(t, seeds, topic, unique("group"), taker.as("a"))
	taker.waitFor(t, 30-2)
	if err := stop(); err != nil {
		t.Fatalf("Follow returned %v; want nil once stopped", err)
	}

	for p, end := range ends {
		from := int64(0)
		if p == resumed {
			from = 2
		}
		if got := taker.offsets(p); !slices.Equal(got, span(from, end)) {
			t.Errorf("partition %d: took %v; want %v, each once and in order", p, got, span(from, end))
		}
	}
}

func TestFollow_twoFollowersInAGroupTakeEveryRecordOnce(t *testing.T) {
	seeds := brokers(t)
	topic := createTopic(t, seeds, 3)
	group := unique("group")
	taker := newFakeTaker()

	stopA := follow(t, seeds, topic, group, taker.as("a"))
	appendRecords(t, seeds, topic, 0, 30)
	taker.waitFor(t, 30)

	stopB := follow(t, seeds, topic, group, taker.as("b"))
	taker.waitAssigned(t, "b")
	appendRecords(t, seeds, topic, 30, 30)
	taker.waitFor(t, 60)
	if err := errors.Join(stopA(), stopB()); err != nil {
		t.Fatalf("Follow returned %v; want nil once stopped", err)
	}

	ends := partitionEnds(t, seeds, topic, 60)
	for p, end := range ends {
		if got := taker.offsets(p); !slices.Equal(got, span(0, end)) {
			t.Errorf("partition %d: took %v; want %v, each once and in order", p, got, span(0, end))
		}
	}
	if taker.count("b") == 0 {
		t.Error("the second follower took nothing; the group did not share the partitions")
	}
}

func TestFollow_stopsWithTheTakersError(t *testing.T) {
	seeds := brokers(t)
	topic := createTopic(t, seeds, 1)
	appendRecords(t, seeds, topic, 0, 1)
	broken := errors.New("the database is gone")

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	err := journal.Follow(ctx, seeds, topic, unique("group"), failingTaker{broken})

	if !errors.Is(err, broken) {
		t.Fatalf("Follow returned %v; want the taker's error", err)
	}
}

func appendRecords(t *testing.T, seeds []string, topic string, from, n int) {
	t.Helper()
	j, err := journal.NewKafka(seeds, topic)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	records := make([]journal.Record, 0, n)
	for i := from; i < from+n; i++ {
		source := "plant-" + strconv.Itoa(i%10) + ".erp"
		records = append(records, journal.Record{Source: source, Body: []byte(`{"n":` + strconv.Itoa(i) + `}`)})
	}
	if err := j.Append(t.Context(), records); err != nil {
		t.Fatal(err)
	}
}

// partitionEnds is the offset one past the last record of every partition
// holding any of the n records in topic.
func partitionEnds(t *testing.T, seeds []string, topic string, n int) map[int32]int64 {
	t.Helper()
	ends := map[int32]int64{}
	for _, r := range consume(t, seeds, topic, n) {
		ends[r.Partition] = max(ends[r.Partition], r.Offset+1)
	}
	return ends
}

func follow(t *testing.T, seeds []string, topic, group string, taker journal.Taker) func() error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- journal.Follow(ctx, seeds, topic, group, taker) }()
	return func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(15 * time.Second):
			t.Fatal("Follow did not stop")
			return nil
		}
	}
}

func span(from, end int64) []int64 {
	s := make([]int64, 0, end-from)
	for o := from; o < end; o++ {
		s = append(s, o)
	}
	return s
}

func unique(prefix string) string {
	return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

// fakeTaker keeps the next offset of every partition and what was taken, in
// memory, shared by every follower it is handed to.
type fakeTaker struct {
	mu       sync.Mutex
	next     map[int32]int64
	taken    map[int32][]int64
	by       map[string]int
	assigned map[string]bool
}

func newFakeTaker() *fakeTaker {
	return &fakeTaker{next: map[int32]int64{}, taken: map[int32][]int64{}, by: map[string]int{}, assigned: map[string]bool{}}
}

func (f *fakeTaker) as(name string) journal.Taker { return namedTaker{f, name} }

func (f *fakeTaker) offsets(p int32) []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.taken[p])
}

func (f *fakeTaker) count(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.by[name]
}

func (f *fakeTaker) waitFor(t *testing.T, n int) {
	t.Helper()
	f.wait(t, func() bool {
		total := 0
		for _, offsets := range f.taken {
			total += len(offsets)
		}
		return total >= n
	}, strconv.Itoa(n)+" records taken")
}

func (f *fakeTaker) waitAssigned(t *testing.T, name string) {
	t.Helper()
	f.wait(t, func() bool { return f.assigned[name] }, name+" assigned partitions")
}

func (f *fakeTaker) wait(t *testing.T, cond func() bool, what string) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		f.mu.Lock()
		ok := cond()
		f.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

type namedTaker struct {
	*fakeTaker
	name string
}

func (n namedTaker) Next(_ context.Context, _ string, partitions []int32) (map[int32]int64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.assigned[n.name] = true
	next := map[int32]int64{}
	for _, p := range partitions {
		if o, ok := n.next[p]; ok {
			next[p] = o
		}
	}
	return next, nil
}

func (n namedTaker) Take(_ context.Context, _ string, partition int32, entries []journal.Entry) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, e := range entries {
		n.taken[partition] = append(n.taken[partition], e.Offset)
		n.next[partition] = e.Offset + 1
		n.by[n.name]++
	}
	return nil
}

type failingTaker struct{ err error }

func (failingTaker) Next(context.Context, string, []int32) (map[int32]int64, error) {
	return map[int32]int64{}, nil
}

func (f failingTaker) Take(context.Context, string, int32, []journal.Entry) error {
	return f.err
}
