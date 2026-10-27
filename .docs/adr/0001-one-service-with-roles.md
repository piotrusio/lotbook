# ADR-0001: Lotbook is one service, run in roles

- **Status:** Accepted
- **Date:** 2026-10-27
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

Lotbook takes raw bookings from ERP, WMS and MES, interprets what each one
means for a lot, keeps the genealogy as a graph and serves it. Those are
several responsibilities with different load profiles: accepting data,
interpreting it in order, answering traversals, and feeding analytical
consumers.

Weighed and rejected:

- **A service per responsibility** (ingest, interpretation, graph, query,
  export). Every service is one more deployable to version, upgrade and
  validate. The genealogy has to be one consistent graph; splitting
  interpretation from the graph across services and stores trades a
  transaction for a distributed consistency problem.
- **One process doing everything.** Ingest, interpretation and heavy
  traversals would compete for the same resources and scale together, and a
  slow analytical query would slow down accepting data.

Connectors to the source systems run next to those systems, carry their own
dependencies — some vendor SDKs may not be redistributed — and change on
their own cadence.

## Decision

**Lotbook is one Go codebase, one module and one image, started in roles.**
The same binary runs as `ingest`, `worker` or `query` (and further roles as
they appear); in Kubernetes each role is its own workload with its own
replica count. Inside, modules are the boundaries.

**Connectors are not part of this repository.** They live in their own
repositories and reach Lotbook only through its versioned ingest contract
over HTTPS — whatever their source is: an ERP database, a vendor API, a
message broker. Nothing else is exposed outside the cluster.

## Consequences

- `+` One artifact to build, version and release; at runtime several
  workloads that scale and fail independently.
- `+` A connector for a new source needs no change to Lotbook.
- `+` The ingest contract is the one inbound interface to keep stable, and it
  is kept in this repository.
- `-` Roles share a codebase, so a module boundary violated in code is not
  stopped by a network boundary. It has to be held in review.
- `-` The ingest contract is public and versioned from its first release;
  changing it is a new version, not an edit.
