# Architecture

> **Designed, not built.** This describes the target the ADRs fix. Of it, only
> the `cmd/lotbook` entry point exists today.

## Context

```text
 ERP / WMS / MES / broker                                      consumers across the company
        │                                                      ▲        ▲          ▲
        ▼                                                      │        │          │
  connectors (own repos) ──HTTPS──▶ ingress ──▶ lotbook ──▶ traversal  change    SQL
                                                  │          API       feed     (read schema,
                                                  ▼                              DB replica)
                                              PostgreSQL
                         └──────────────── one Kubernetes cluster ────────────────┘
```

Only HTTPS crosses the cluster's edge: the ingest contract inbound, the
traversal API and the change feed outbound. PostgreSQL is reachable from
Lotbook's pods only, except the read replica exposed for SQL consumers.

## Roles

One image, started in roles ([ADR-0001](../adr/0001-one-service-with-roles.md)),
each its own workload:

| Role | Does | Scales |
|---|---|---|
| `ingest` | accepts batches of source row changes over HTTPS, writes them to the journal, answers `202` once they are durable | stateless, any number of replicas |
| `worker` | follows the journal in commit order: maintains the mirror, assembles documents, interprets them and writes genealogy events | replicas share the work by key |
| `query` | holds the graph in memory, serves traversals with their documents, and the change feed | stateful set, every replica holds the whole graph |

## Data flow

```text
connector: source row changes (table, operation, key, row, transaction, commit position)
   ▼
journal ─────────────▶ mirror ──────────────▶ documents ──────────────▶ interpretation
(record: the source's   (derived: current      (derived: assembled         │
 transaction log,        state of the source's  from complete source        ▼
 commit order)           tables, per source)    transactions, versioned)  genealogy events (seq)
                                                       │                 (record, with provenance)
                                                       │                    │
                                                       │                    ├──▶ graph tables + read schema
                                                       │                    ├──▶ CSR in every query replica
                                                       │                    └──▶ change feed
                                                       ▼                    ▼
                                                  query engine: traverse the graph, fetch the documents
```

The journal and the genealogy events are the records; the mirror, the
documents and the graph are derived and can be rebuilt
([ADR-0004](../adr/0004-the-journal-and-genealogy-events-are-the-records.md)).
How a document is assembled from a source's tables, and what it means for a
lot, are both specific to the source system.
Everything downstream of the genealogy events follows one sequence and states
the one it is current to
([ADR-0005](../adr/0005-the-genealogy-is-served-three-ways.md)).

## The graph

The skeleton — node ids, kinds, times, edges in both directions — is held in
memory as CSR with a delta for real-time changes and background compaction.
Nodes carry document keys; the documents themselves stay in PostgreSQL and are
fetched for a result only
([ADR-0006](../adr/0006-the-graph-lives-in-memory-as-csr.md)).

Memory per `query` replica is about `8·E + 17·N` bytes for E edges and N
nodes: 100 million nodes at 5 edges each is about 5.7 GB.

## Tracing

Every asynchronous step starts a trace of its own and links to the traces of
what it read; the context crosses each boundary as data
([ADR-0008](../adr/0008-the-trace-context-travels-with-the-journal.md)):

```text
connector ──traceparent──▶ ingest ──▶ journal (trace context per change)
                                         ┆ link
                                      assembly
                                         ┆ link
                                   interpretation ──▶ genealogy event (trace context)
                                                             ┆ link
                                                  query replica, change feed
```

`──▶` writes, `┆ link` is a span link from the later step to the earlier one.

W3C Trace Context on the wire, OTLP over HTTP out. Logs are `log/slog` JSON
on standard output, each line carrying `trace_id` and `span_id`.

## Dependencies

Lotbook's code imports the Go standard library,
[pgx](https://github.com/jackc/pgx) and
[OpenTelemetry](https://opentelemetry.io/)'s API, SDK and OTLP/HTTP exporter —
nothing else
([ADR-0007](../adr/0007-the-standard-library-postgresql-and-opentelemetry.md)).
`depguard` in [`.golangci.yml`](../../.golangci.yml) refuses any other import.
HTTP and pgx instrumentation is our own: middleware on the server and pgx's
tracer hooks.

Beneath the OTLP exporter come the protocol's generated types and with them
protobuf, gRPC, grpc-gateway and a few `golang.org/x` modules — some twenty in
the binary, the same over HTTP as over gRPC. `go version -m bin/lotbook` lists
them; `make vuln` checks all of them.

## Platform

Lotbook runs on Kubernetes, at least three worker nodes
([ADR-0002](../adr/0002-kubernetes-is-the-platform.md)), and installs with:

- **PostgreSQL** through the CloudNativePG operator — three instances,
  streaming replication, automatic failover, backups
  ([ADR-0003](../adr/0003-postgresql-not-a-message-broker.md));
- **graph snapshots** on each `query` replica's persistent volume, copied
  between replicas when a new one starts — no object store.

Installed with a Helm chart. Migrations, compaction and snapshots are
explicit commands of the binary. A single-node install is for evaluation and
development only.
