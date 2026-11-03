// Package journal is the record of what each source said (ADR-0013), kept in
// a Kafka topic partitioned by source (ADR-0009).
package journal

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Record is one source record on its way into the journal: its source, its
// body exactly as the connector sent it, the contract it was accepted under
// and the trace context of the request that accepted it (ADR-0008, ADR-0014).
type Record struct {
	Source      string
	Body        []byte
	Contract    string
	TraceParent string
}

// Kafka appends records to the journal's topic, keyed by source, so that all
// of a source's records share a partition and keep their order. It never
// creates the topic: its configuration is the installation's (ADR-0016).
type Kafka struct {
	client *kgo.Client
}

// NewKafka makes a journal writer for topic on the cluster reached through
// seeds. It connects when it first appends.
func NewKafka(seeds []string, topic string) (*Kafka, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.DefaultProduceTopic(topic),
	)
	if err != nil {
		return nil, fmt.Errorf("journal: %w", err)
	}
	return &Kafka{client: client}, nil
}

// Append returns once every record is durable in the journal, or with the
// first error when any is not. Records it reports failed may still have been
// written; the workers drop a record seen twice by its position (ADR-0013).
func (k *Kafka) Append(ctx context.Context, records []Record) error {
	rs := make([]*kgo.Record, 0, len(records))
	for _, r := range records {
		rs = append(rs, &kgo.Record{
			Key:   []byte(r.Source),
			Value: r.Body,
			Headers: []kgo.RecordHeader{
				{Key: "lotbook-contract", Value: []byte(r.Contract)},
				{Key: "traceparent", Value: []byte(r.TraceParent)},
			},
		})
	}
	if err := k.client.ProduceSync(ctx, rs...).FirstErr(); err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	return nil
}

// Close closes the connections; Append leaves nothing buffered to flush.
func (k *Kafka) Close() {
	k.client.Close()
}
