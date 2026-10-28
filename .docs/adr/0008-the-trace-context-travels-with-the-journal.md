# ADR-0008: The trace context travels with the journal and the genealogy events

- **Status:** Proposed
- **Date:** 2026-10-28
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

Almost nothing Lotbook does happens inside the request that caused it. `ingest`
answers `202` once a batch is durable in the journal; a `worker` assembles and
interprets it later, in commit order (ADR-0003, ADR-0004); the `query`
replicas apply the resulting genealogy events later still, and the change feed
hands them on (ADR-0005, ADR-0006). A trace that lives as long as a request
ends at the `202`, and everything after it is invisible — which is where every
interesting delay and every wrong link is made.

The steps do not line up one to one. One source transaction can arrive over
several batches, one document is assembled from several transactions, and one
interpretation writes several genealogy events. A step has many causes, each
in another request, and they are minutes apart.

Weighed and rejected:

- **Continuing the ingest trace in the worker, as its child.** A step with
  several causes can have only one parent, so the choice of parent would be
  arbitrary, and a trace held open across minutes of asynchronous work has no
  natural end.
- **Correlating by the data alone** — searching for the spans that touched a
  given journal position. It works only while every span carries every key
  it touched, and it gives an investigation nothing to start from.

## Decision

**The trace context is carried across every asynchronous boundary in the
pipeline, as data stored next to the records, in the W3C Trace Context
format.**

- **The ingest contract accepts a trace context** from the connector. When one
  is sent, the ingest span continues it; when none is, ingest starts a trace
  of its own. The contract never requires one.
- **The journal keeps, for every change it accepts, the trace context of the
  ingest request that wrote it.**
- **Each asynchronous step starts a trace of its own and links to the traces of
  what it read** — assembling a document links to the ingest traces of its
  journal changes; interpreting it links to the assembly. A link, not a
  parent: a step with many causes keeps all of them.
- **Every genealogy event keeps the trace context of the interpretation that
  wrote it**, next to its provenance. Applying the event in a `query`
  replica and handing it on through the change feed link to it.
- **The trace context is observability data, never input.** Interpretation
  does not read it and nothing it decides depends on it; it is stored whether
  or not the trace was sampled, so the ids in the logs (ADR-0007) can still
  be followed.

## Consequences

- `+` From any edge in the graph to the interpretation that wrote it, to the
  documents it read, to the connector's request that delivered the rows —
  the same path the provenance gives an auditor, open to an operator.
- `+` A connector that is traced itself is joined to Lotbook's traces with no
  agreement beyond an open standard.
- `+` The time from a change committed in the source to its edge in the graph
  becomes measurable, step by step.
- `-` The journal and the genealogy events each carry a trace context per
  record — some 25 bytes.
- `-` The path is told by span links, which an observability backend has to
  show to be of use.
