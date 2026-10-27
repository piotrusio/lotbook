# ADR-0002: Lotbook runs on Kubernetes and brings its dependencies into the cluster

- **Status:** Accepted
- **Date:** 2026-10-27
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

Lotbook is fed in real time and read by many consumers at once, so an
installation has to keep serving through the loss of a node. It has stateful
parts — a database and the graph held by its query replicas — that must
survive failures, and upgrades that change the database schema in a
controlled order.

Weighed and rejected:

- **Plain virtual machines.** Availability, failover, rolling upgrades and
  replica scaling would each have to be built into Lotbook for every way a
  set of machines can be arranged.
- **Stateful dependencies provided from outside the installation** (an
  existing database server, an object store). Lotbook's behaviour would then
  depend on how each of them is configured, sized and upgraded, outside
  anything Lotbook's install can set or check.
- **A single-node layout for production.** It does not survive the loss of
  its node.

## Decision

**Lotbook runs on Kubernetes**, on a cluster of at least three worker nodes.
Which distribution, and whether managed or self-run, is not constrained.

**The installation brings its stateful dependencies into the cluster.**
PostgreSQL runs through a Kubernetes operator installed with Lotbook, as
several instances with replication and automatic failover. No other stateful
dependency is required (ADR-0003, ADR-0006).

**Lotbook is installed with a Helm chart.** Life-cycle steps that must happen
in order — schema migrations, graph compaction and snapshots — are explicit
commands of the binary, so that they can be run by hand or driven by
automation.

A single-node install is for evaluation and development only.

## Consequences

- `+` Availability, failover and rolling upgrades come from the platform
  rather than from Lotbook.
- `+` An installation is self-contained: its database is configured, sized
  and upgraded by the install itself.
- `-` Running Lotbook requires a Kubernetes cluster.
- `-` The installation operates PostgreSQL and owns its configuration,
  backups and upgrades.
