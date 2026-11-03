# ADR-0016: Lotbook runs on Kubernetes and takes PostgreSQL and Kafka from the environment

- **Status:** Accepted
- **Date:** 2026-11-03
- **Supersedes:** ADR-0010
- **Superseded-by:** —
- **Issue/PR:** —

## Context

ADR-0010 brought PostgreSQL and Kafka into the installation through
operators installed with Lotbook, and refused databases provided from outside
it: Lotbook's behaviour would depend on configuration its install could
neither set nor check.

The journal is a record kept for ever (ADR-0011, ADR-0013), and the genealogy
events are the other record. A record has to outlive every release of the
application that writes it. Databases installed with an application share
that application's life cycle: an uninstall, or a wrong value in an upgrade,
reaches the record.

A regulated company runs PostgreSQL and Kafka already, as a platform of its
own, with the backups, monitoring, upgrades and validation its practice
requires. A second copy of each, installed beside it by one application, is
outside that practice and is refused there.

What Lotbook relies on in either is narrow: a few features of a version, the
configuration of one topic, the rights of its own identity. All of it can be
read from a running system, so ADR-0010's objection is answered by checking,
not by owning.

Weighed and rejected:

- **PostgreSQL and Kafka installed with Lotbook through operators**
  (ADR-0010). The record shares the life cycle of an application release, and
  the company gets a second copy of systems it already runs.
- **An optional PostgreSQL and Kafka in the chart, for evaluation.** A second
  way to install, kept working and tested, that is never production.
  Evaluation runs them locally, beside the binary.
- **Taking the environment's configuration on trust.** A topic that is
  compacted or whose retention expires erases the record silently, and
  nothing would say so until the history is needed.

## Decision

**Lotbook runs on Kubernetes, on a cluster of at least three worker nodes,
and takes PostgreSQL, Kafka and object storage from the environment.** Which
distribution, and whether each is managed or self-run, is not constrained.
The chart installs Lotbook and nothing else.

- **PostgreSQL:** several instances with replication and automatic failover;
  a database and a role that owns it, for Lotbook; a read replica exposed to
  SQL consumers (ADR-0005).
- **Kafka:** at least three brokers with replication; tiered storage enabled
  on the brokers, moving closed segments to object storage over the S3 API
  (ADR-0011); an identity for Lotbook that may produce to and consume the
  journal's topic, as one consumer group.
- **Object storage** is used by Kafka's tiered storage, not by Lotbook. Its
  durability is the environment's to provide.
- **Lotbook creates what is its own** — its schema and the journal's topic —
  with its migration command. The topic is kept for ever and never compacted
  (ADR-0011), replicated three times, with at least two replicas in sync for a
  write to count, and in tiered storage. Where the environment does not let
  Lotbook create the topic, a topic configured exactly so is the requirement.
- **Lotbook checks what it relies on**: the versions, the topic's
  configuration and its own rights. A command of the binary checks them
  before an install; every role checks them at start and is not ready while
  one fails, naming what it found and what it requires.
- **The minimum versions are those whose features Lotbook uses**, stated with
  the chart and checked as above.

Life-cycle steps that must happen in order — schema migrations, graph
compaction and snapshots — are explicit commands of the binary, so that they
can be run by hand or driven by automation. Lotbook is installed with a Helm
chart.

Evaluation and development run without Kubernetes, with PostgreSQL and Kafka
started locally beside the binary.

## Consequences

- `+` The records outlive any release of Lotbook; removing or reinstalling
  the application does not reach them.
- `+` PostgreSQL and Kafka are run the way the company already runs them,
  inside its own backups, monitoring and validation.
- `+` The chart is the application alone.
- `+` A misconfigured environment is refused by name, before the first record
  is accepted.
- `-` Running Lotbook requires a Kubernetes cluster, PostgreSQL, a Kafka with
  tiered storage, and object storage behind it.
- `-` The durability of the records is the environment's: the replication of
  PostgreSQL and Kafka and the protection of the bucket are requirements of
  the installation, which Lotbook checks where it can and cannot set.
- `-` A configuration changed after Lotbook started is caught at the next
  start, not at the moment it changes.
