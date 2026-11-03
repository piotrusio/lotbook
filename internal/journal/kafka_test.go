package journal_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"lotbook.org/lotbook/internal/journal"
)

func TestKafka_appendsRecordsKeyedBySourceInTheirOrder(t *testing.T) {
	seeds := brokers(t)
	topic := createTopic(t, seeds, 3)
	j, err := journal.NewKafka(seeds, topic)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	trace := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	records := make([]journal.Record, 0, 12)
	for i := range cap(records) {
		source := []string{"plant-a.erp", "plant-b.mes", "plant-c.wms"}[i%3]
		records = append(records, journal.Record{
			Source:      source,
			Body:        []byte(`{"source":"` + source + `","n":` + strconv.Itoa(i) + `}`),
			Contract:    "ingest/v1",
			TraceParent: trace,
		})
	}
	if err := j.Append(t.Context(), records); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got := consume(t, seeds, topic, len(records))
	partition := map[string]int32{}
	bodies := map[string][]string{}
	for _, r := range got {
		source := string(r.Key)
		if p, seen := partition[source]; seen && p != r.Partition {
			t.Errorf("source %s is in partitions %d and %d; want one", source, p, r.Partition)
		}
		partition[source] = r.Partition
		bodies[source] = append(bodies[source], string(r.Value))
		headers := map[string]string{}
		for _, h := range r.Headers {
			headers[h.Key] = string(h.Value)
		}
		if headers["lotbook-contract"] != "ingest/v1" || headers["traceparent"] != trace {
			t.Errorf("headers = %v; want the contract and the traceparent", headers)
		}
	}
	for _, r := range records {
		want := string(r.Body)
		if len(bodies[r.Source]) == 0 || bodies[r.Source][0] != want {
			t.Fatalf("source %s: next in the journal is %v; want %s, in the order appended", r.Source, bodies[r.Source], want)
		}
		bodies[r.Source] = bodies[r.Source][1:]
	}
}

func TestKafka_failsWhenTheTopicDoesNotExist(t *testing.T) {
	seeds := brokers(t)
	j, err := journal.NewKafka(seeds, "lotbook-test-missing-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := j.Append(ctx, []journal.Record{{Source: "plant-a.erp", Body: []byte(`{}`)}}); err == nil {
		t.Fatal("Append succeeded into a topic that does not exist; the journal must not create it")
	}
}

func brokers(t *testing.T) []string {
	t.Helper()
	seeds := os.Getenv("LOTBOOK_TEST_KAFKA_BROKERS")
	if seeds == "" {
		t.Skip("LOTBOOK_TEST_KAFKA_BROKERS is not set; this test needs a Kafka")
	}
	return strings.Split(seeds, ",")
}

func createTopic(t *testing.T, seeds []string, partitions int32) string {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(seeds...))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	name := "lotbook-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	topic := kmsg.NewCreateTopicsRequestTopic()
	topic.Topic = name
	topic.NumPartitions = partitions
	topic.ReplicationFactor = 1
	req := kmsg.NewPtrCreateTopicsRequest()
	req.Topics = append(req.Topics, topic)
	resp, err := req.RequestWith(t.Context(), cl)
	if err != nil {
		t.Fatal(err)
	}
	if err := kerr.ErrorForCode(resp.Topics[0].ErrorCode); err != nil {
		t.Fatalf("create topic %s: %v", name, err)
	}
	return name
}

func consume(t *testing.T, seeds []string, topic string, n int) []*kgo.Record {
	t.Helper()
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	var got []*kgo.Record
	for len(got) < n {
		fetches := cl.PollFetches(ctx)
		if ctx.Err() != nil {
			t.Fatalf("read %d records of %d from the journal", len(got), n)
		}
		fetches.EachRecord(func(r *kgo.Record) { got = append(got, r) })
	}
	return got
}
