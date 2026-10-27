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
| `ingest` | accepts batches of raw records over HTTPS, writes them to the journal, answers `202` once they are durable; idempotent by the source's record id | stateless, any number of replicas |
| `worker` | interprets the journal in order and writes genealogy events | replicas share the work by key |
| `query` | holds the graph in memory, serves traversals and the change feed | stateful set, every replica holds the whole graph |

## Data flow

```text
raw records ──▶ journal ──▶ interpretation ──▶ genealogy events (seq) ──┬──▶ graph tables + read schema
              (immutable)    (rules, versioned)  (immutable, provenance)  ├──▶ CSR in every query replica
                                                                         └──▶ change feed
```

The journal and the genealogy events are the records; the graph is derived
from the events and can be rebuilt
([ADR-0004](../adr/0004-genealogy-events-are-the-source-of-truth.md)).
Everything downstream follows one sequence and states the one it is current to
([ADR-0005](../adr/0005-the-genealogy-is-served-three-ways.md)).

## The graph

The skeleton — node ids, kinds, times, edges in both directions — is held in
memory as CSR with a delta for real-time changes and background compaction;
document data stays in PostgreSQL and is joined for results only
([ADR-0006](../adr/0006-the-graph-lives-in-memory-as-csr.md)).

Memory per `query` replica is about `8·E + 17·N` bytes for E edges and N
nodes: 100 million nodes at 5 edges each is about 5.7 GB.

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
