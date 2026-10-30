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
                                      Kafka (journal), PostgreSQL
                         └──────────────── one Kubernetes cluster ────────────────┘
```

Only HTTPS crosses the cluster's edge: the ingest contract inbound
(`POST /v1/source-events`,
[ADR-0014](../adr/0014-the-ingest-contract-accepts-persists-and-answers.md),
contract in [`contracts/ingest/v1/openapi.json`](../../contracts/ingest/v1/openapi.json)),
the traversal API and the change feed outbound. Kafka and PostgreSQL are
reachable from Lotbook's pods only, except the PostgreSQL read replica
exposed for SQL consumers.

## Roles

One image, started in roles ([ADR-0001](../adr/0001-one-service-with-roles.md)),
each its own workload:

| Role | Does | Scales |
|---|---|---|
| `ingest` | accepts source records — row changes or events — over HTTPS, writes them to the journal in Kafka, answers `202` once Kafka holds them durably | stateless, any number of replicas |
| `worker` | follows the journal in each source's order: maintains the mirror of relational sources, assembles documents, interprets them and writes genealogy events | replicas share the journal's partitions as one consumer group |
| `query` | holds the graph in memory, serves traversals with their documents, and the change feed | stateful set, every replica holds the whole graph |

## Identity and access

Every HTTPS request carries a bearer token from the installation's IdP —
Entra ID, Okta, Google, Keycloak, any OpenID Connect provider. Lotbook trusts
the issuers it is configured with, verifies tokens against their published
keys itself, and takes from a token only its issuer and subject. What that
identity may do is Lotbook's mapping, in PostgreSQL
([ADR-0015](../adr/0015-identity-from-the-idp-authorization-in-lotbook.md)):

| Interface | Used by | May |
|---|---|---|
| ingest | one service account per connector | write records of the sources mapped to it |
| traversal API | systems and people | read the genealogy, given the role |
| change feed | systems | read the feed, given the role |
| SQL read schema | analytical tools | PostgreSQL roles, read-only views |
| life-cycle commands | operators | Kubernetes RBAC |

## Data flow

```text
connector: source records (source, position, unit; a row change or an event)
   ▼
journal ──row changes──▶ mirror ────────────▶ documents ──────────────▶ interpretation
(record: what the      (derived: current        ▲ (derived: assembled      │
 source said, in its    state of a relational   │  from complete units,    ▼
 order; Kafka, a        source's tables)        │  versioned)            genealogy events (seq)
 partition per source)                          │
   └──────────────events────────────────────────┘
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
([ADR-0013](../adr/0013-the-journal-holds-what-the-source-said.md)).
A source is one ordered stream; its records are row changes, which go
through the mirror, or events, which are assembled into documents directly.
The journal lives in Kafka, the genealogy events and everything served from
them in PostgreSQL; a worker commits the genealogy events it writes together
with the journal position it consumed, in one PostgreSQL transaction
([ADR-0009](../adr/0009-the-journal-is-a-kafka-log-partitioned-by-source.md)).
How a document is assembled from a source's records, and what it means for a
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
connector ──traceparent──▶ ingest ──▶ journal (trace context per record)
                                         ┆ link
                                      assembly
                                         ┆ link
                                   interpretation ──▶ genealogy event (trace context)
                                                             ┆ link
                                                  query replica, change feed
```

`──▶` writes, `┆ link` is a span link from the later step to the earlier one.
A span is one unit of work — a request, a document, an interpretation, a run of
events applied, a feed response — and links once per distinct trace it read,
up to the SDK's link limit, recording how many it left out. The journal and
the genealogy events store the `traceparent` only; the change feed hands it on
to consumers.

W3C Trace Context on the wire, OTLP over HTTP out. Logs are `log/slog` JSON
on standard output, each line carrying `trace_id` and `span_id`.

## Dependencies

Lotbook's code imports the Go standard library,
[pgx](https://github.com/jackc/pgx),
[OpenTelemetry](https://opentelemetry.io/)'s API, SDK and OTLP/HTTP exporter
([ADR-0007](../adr/0007-the-standard-library-postgresql-and-opentelemetry.md))
and [franz-go](https://github.com/twmb/franz-go)'s client and admin package
([ADR-0012](../adr/0012-a-pure-go-kafka-client.md)) — nothing else.
`depguard` in [`.golangci.yml`](../../.golangci.yml) refuses any other import,
and franz-go's plugins. HTTP, pgx and Kafka instrumentation is our own:
middleware on the server, pgx's tracer hooks, franz-go's hooks.

Beneath the OTLP exporter come the protocol's generated types and with them
protobuf, gRPC, grpc-gateway and a few `golang.org/x` modules — some twenty in
the binary, the same over HTTP as over gRPC. `go version -m bin/lotbook` lists
them; `make vuln` checks all of them. franz-go adds three: its protocol
messages and the `klauspost/compress` and `lz4` codecs.

## Platform

Lotbook runs on Kubernetes, at least three worker nodes
([ADR-0010](../adr/0010-kubernetes-with-an-object-store-from-the-environment.md)),
and installs with:

- **PostgreSQL** through the CloudNativePG operator — three instances,
  streaming replication, automatic failover, backups;
- **Kafka** through the Strimzi operator — at least three brokers in KRaft
  mode, the journal replicated across them
  ([ADR-0009](../adr/0009-the-journal-is-a-kafka-log-partitioned-by-source.md));
  the journal's topic with unlimited retention, no compaction, and tiered
  storage moving closed segments to object storage through an S3 remote
  storage plugin in the broker image
  ([ADR-0011](../adr/0011-the-journal-is-kept-for-ever-through-tiered-storage.md));
- **graph snapshots** on each `query` replica's persistent volume, copied
  between replicas when a new one starts.

It takes from the environment one thing: an **S3-compatible object store**
for the journal's cold segments — a cloud provider's, or one run on premises.

Installed with a Helm chart. Migrations, compaction and snapshots are
explicit commands of the binary. A single-node install is for evaluation and
development only.
