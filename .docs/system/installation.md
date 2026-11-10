# Installation

> **A walking skeleton.** This version installs `ingest`, `worker` and the
> `migrate` Job: records sent to `POST /v1/source-events` reach the journal
> and PostgreSQL. It does not yet authenticate senders, speak TLS or SASL to
> Kafka, create the journal's topic, or check the environment beyond the
> schema — each is marked where it matters below. **Do not expose `ingest`
> outside a network you trust until it authenticates.**

Lotbook takes PostgreSQL, Kafka and object storage from the environment, and
its Helm chart installs Lotbook alone
([ADR-0016](../adr/0016-kubernetes-with-postgresql-and-kafka-from-the-environment.md)).
The environment adapts to what is written here; Lotbook does not adapt to the
environment.

## What the environment provides

### Kubernetes

- A release still supported upstream. The chart uses no API newer than
  Kubernetes 1.21; it is tested on 1.34.
- At least three worker nodes. The pods of a role are spread across nodes,
  and a PodDisruptionBudget keeps all but one of them running through a
  drain.
- Pods that meet the `restricted` Pod Security Standard are enough: every
  Lotbook pod runs as a non-root user with a read-only root filesystem, no
  capabilities, the runtime's default seccomp profile and no service-account
  token. Tested with `restricted` enforced on the namespace.
- The image, `ghcr.io/piotrusio/lotbook`, pullable from the cluster — public,
  for `linux/amd64` and `linux/arm64`, tagged with the full git sha of the
  commit it was built from — or a mirror of it.
- To reach `ingest` from outside the cluster: an Ingress controller or a
  Gateway API implementation, terminating TLS. The chart routes `/v1/` only.

### PostgreSQL

- A release still supported upstream. Lotbook uses nothing newer than
  PostgreSQL 9.5; it is tested on 17.
- Several instances with replication, automatic failover and backups:
  PostgreSQL holds the genealogy events, a record nothing can rebuild
  (ADR-0013).
- A database, and a role that owns it: `migrate` creates its tables in the
  first schema of the role's `search_path`.
- The connection URL, in libpq's URL form, in a Secret in the release's
  namespace (key `url` by default). Only `worker` and `migrate` are given it.
  - Each of their pods opens up to the greater of 4 and the node's CPU count
    connections; `pool_max_conns` in the URL sets another bound.
  - `sslmode=require` encrypts the connection. Verifying the server's
    certificate against a private CA needs the CA in the pod, which the chart
    does not mount yet.
- A read replica for SQL consumers (ADR-0005). Nothing uses it yet.

### Kafka

- Kafka 3.9 or newer — the first release in which tiered storage is
  production-ready. Tested on 4.1.
- At least three brokers.
- Tiered storage enabled on the brokers, with a remote storage manager for
  an S3-compatible object store. Apache Kafka does not ship one; it is part of
  the Kafka the environment provides (ADR-0011).
- A record of up to the topic's `max.message.bytes` (about 1 MB by default);
  a larger one is refused with `503` until the topic takes it.
- **Plaintext only, for now.** Lotbook does not yet speak TLS or SASL to Kafka,
  so the brokers must be reachable from Lotbook's pods without either — inside
  the cluster, behind a network policy.
- Where the brokers enforce ACLs, Lotbook's principal may write to and
  describe the journal's topic (`ingest`), and read and describe it and read
  its consumer group, `lotbook.worker` by default (`worker`).

### The journal's topic

**Created by the environment, before Lotbook is installed.** ADR-0016 has
Lotbook's migration create it; this version does not yet.

| Setting | Value | Why |
|---|---|---|
| name | `lotbook.journal`, or the chart's `kafka.journalTopic` | |
| `cleanup.policy` | `delete` | compaction would erase the record ([ADR-0011](../adr/0011-the-journal-is-kept-for-ever-through-tiered-storage.md)) |
| `retention.ms` | `-1` | kept for ever |
| `retention.bytes` | `-1` | kept for ever |
| `remote.storage.enable` | `true` | closed segments move to object storage |
| `local.retention.ms` | the hot window, the environment's choice | what the brokers keep on their own disks |
| replication factor | `3` | |
| `min.insync.replicas` | `2` | a write counts once two replicas hold it |
| partitions | chosen once, **never raised** | see below |

A source's records share a partition and keep their order there
([ADR-0009](../adr/0009-the-journal-is-a-kafka-log-partitioned-by-source.md)).
Raising the partition count moves sources to other partitions, and a source's
records written after the change are no longer ordered after those written
before it. The count is therefore fixed at creation, with room for the largest
number of workers the installation will run: a partition is taken by one
worker at a time, so workers beyond the partition count stand idle.

### Object storage

An S3-compatible store behind Kafka's tiered storage — a cloud provider's or
one run on premises. Lotbook never addresses it. Its durability is the
journal's durability past the hot window, and its replication and protection
are the environment's.

### Observability

Logs are JSON lines on stdout, each written inside a trace carrying its
`trace_id` and `span_id`. Traces go out over OTLP/HTTP when
`OTEL_EXPORTER_OTLP_ENDPOINT` is set, through the chart's `env`; the other
standard `OTEL_*` variables apply.

## Installing

From a checkout of this repository, the chart is at `deploy/helm/lotbook`. Its
`values.yaml` documents every value; three are required.

```bash
kubectl -n lotbook create secret generic lotbook-postgres --from-literal=url='postgres://lotbook:…@postgres.example:5432/lotbook?sslmode=require'
```

```bash
helm install lotbook deploy/helm/lotbook -n lotbook --set image.tag=<git sha> --set kafka.brokers=kafka-0.kafka:9092,kafka-1.kafka:9092,kafka-2.kafka:9092 --set postgres.existingSecret=lotbook-postgres
```

Before the workloads are applied, the `migrate` Job brings the schema up to
date; the install stops if it fails. Every flag of the binary can be given as
a `LOTBOOK_*` variable, and the chart sets them so: `LOTBOOK_POSTGRES_URL`
reaches the process from the Secret and never appears on a command line.

## Upgrading

`helm upgrade` runs the `migrate` Job again before the new pods roll out; it
changes nothing when there is nothing to migrate. A worker checks at start
that the schema is at exactly its own version, so while an upgrade that
changes the schema rolls out, an old worker that restarts does not start
again until it is replaced by a new one.

## What Lotbook checks

ADR-0016 has Lotbook check, before an install and in every role at start, the
versions, the topic's configuration and its own rights, and stay not ready
while any fails. This version checks one thing: `worker` refuses to start on
a schema that is not at its version. Until the rest is built, the requirements
above are the installation's to meet, and nothing reports when they are not.
