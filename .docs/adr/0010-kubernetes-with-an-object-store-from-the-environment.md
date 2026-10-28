# ADR-0010: Lotbook runs on Kubernetes, brings its databases and takes object storage from the environment

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** ADR-0002
- **Superseded-by:** —
- **Issue/PR:** —

## Context

Lotbook is fed in real time and read by many consumers at once, so an
installation has to keep serving through the loss of a node. It has stateful
parts — PostgreSQL, Kafka and the graph held by its query replicas — that
must survive failures, and upgrades that change the database schema in a
controlled order.

ADR-0002 brought every stateful dependency into the installation and refused
any provided from outside it, an object store included. The journal changes
that: it is a record kept for ever (ADR-0004) in a log that grows without
bound (ADR-0009), read again only rarely — for a rebuild or an audit — and
in sequence. That is cold data, and object storage is where cold data is
kept: written once, read whole, replicated and durable by the store's own
means.

An object store is not a database. It is used through a few operations on
immutable objects, so there is little in how it is configured, sized or
upgraded that could change what Lotbook does — the reason ADR-0002 gave for
refusing outside dependencies holds for a database server, not for a bucket.

Weighed and rejected:

- **Plain virtual machines.** Availability, failover, rolling upgrades and
  replica scaling would each have to be built into Lotbook for every way a
  set of machines can be arranged.
- **Databases provided from outside the installation** — an existing
  PostgreSQL server, an existing Kafka cluster. Lotbook's behaviour would then
  depend on how each of them is configured, sized and upgraded, outside
  anything Lotbook's install can set or check.
- **An object store brought into the cluster.** A third stateful system to
  operate, replicated across nodes and disks, in every installation —
  where the environment already provides one.
- **A single-node layout for production.** It does not survive the loss of
  its node.

## Decision

**Lotbook runs on Kubernetes**, on a cluster of at least three worker nodes.
Which distribution, and whether managed or self-run, is not constrained.

**The installation brings its databases into the cluster.** PostgreSQL and
Kafka run through Kubernetes operators installed with Lotbook, each as
several instances with replication and automatic failover (ADR-0006,
ADR-0009).

**Object storage is provided by the environment**, over the S3 API — a cloud
provider's, or one the company runs on premises. It is the one stateful
dependency the installation does not bring, and its durability is the
environment's to provide.

**Lotbook is installed with a Helm chart.** Life-cycle steps that must happen
in order — schema migrations, graph compaction and snapshots — are explicit
commands of the binary, so that they can be run by hand or driven by
automation.

A single-node install is for evaluation and development only.

## Consequences

- `+` Availability, failover and rolling upgrades come from the platform
  rather than from Lotbook.
- `+` The databases are configured, sized and upgraded by the install itself.
- `+` Cold data goes where the environment already keeps it, without Lotbook
  running storage of its own.
- `-` Running Lotbook requires a Kubernetes cluster and an S3-compatible
  object store.
- `-` The installation operates PostgreSQL and Kafka and owns their
  configuration, backups and upgrades.
- `-` The records kept in object storage are as durable as the environment
  makes them; the bucket's replication and protection are a requirement of
  the installation, not something Lotbook can set.
