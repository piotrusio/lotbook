# ADR-0008: The trace context travels with the journal and the genealogy events

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

Almost nothing Lotbook does happens inside the request that caused it. `ingest`
answers `202` once a request's records are durable in the journal; a `worker`
assembles and interprets them later, in each source's order (ADR-0013,
ADR-0009); the `query` replicas apply the resulting genealogy events later
still, and the change feed hands them on (ADR-0005, ADR-0006). A trace that
lives as long as a request ends at the `202`, and everything after it is
invisible — which is where every interesting delay and every wrong link is
made.

The steps do not line up one to one. One unit of a source — a transaction,
say — can arrive over several requests, one document is assembled from several
units, and one interpretation writes several genealogy events. A step has
many causes, each in another request, and they are minutes apart. Some steps
read at scale: a large unit, a rebuild of the mirror, a corrected rule
reading the journal again (ADR-0013).

Weighed and rejected:

- **Continuing the ingest trace in the worker, as its child.** A step with
  several causes can have only one parent, so the choice of parent would be
  arbitrary, and a trace held open across minutes of asynchronous work has no
  natural end.
- **Correlating by the data alone** — searching for the spans that touched a
  given journal position. It works only while every span carries every key
  it touched, and it gives an investigation nothing to start from.
- **A span per record.** At the rate rows arrive and events are applied, it
  costs more than the work it describes.

## Decision

**The trace context is carried across every asynchronous boundary in the
pipeline, as data stored next to the records, in the W3C Trace Context
format.**

- **The ingest contract accepts a trace context** from the connector. When one
  is sent, the ingest span continues it; when none is, ingest starts a trace
  of its own. The contract never requires one.
- **Every record in the journal can be traced to the ingest request that wrote
  it.**
- **Each asynchronous step starts a trace of its own and links to the traces of
  what it read** — assembling a document links to the ingest traces of its
  journal records; interpreting it links to the assembly. A link, not a
  parent: a step with many causes keeps all of them.
- **Every genealogy event keeps the trace context of the interpretation that
  wrote it**, next to its provenance. Applying events in a `query` replica
  and serving them through the change feed link to it.
- **The change feed hands each event's trace context on**, so a consumer can
  link its own processing to the interpretation that produced the event — the
  same open standard on the way out as on the way in.
- **A span describes a unit of work, never a record:** one request accepted, one
  document assembled, one interpretation, one run of events applied in a
  replica, one response of the change feed.
- **Links are bounded.** A step links once per distinct trace it read, not
  once per record — the records of one request share one. Above a fixed limit
  it links no further and records how many it left out, so a cut list says it
  is cut.
- **What is stored is the `traceparent` alone** — trace id, span id and flags,
  25 bytes. A `tracestate` sent by a connector is carried no further than its
  request.
- **The trace context is observability data, never input.** Interpretation
  does not read it and nothing it decides depends on it; it is stored whether
  or not the trace was sampled, so the ids in the logs (ADR-0007) can still
  be followed.

## Consequences

- `+` From any edge in the graph to the interpretation that wrote it, to the
  documents it read, to the connector's request that delivered the rows —
  the same path the provenance gives an auditor, open to an operator.
- `+` A traced connector and a traced consumer are joined to Lotbook's traces
  with no agreement beyond an open standard.
- `+` The time from a record leaving the source to its edge in the graph
  becomes measurable, step by step.
- `-` The journal and the genealogy events carry 25 bytes of trace context
  per record, or per request where the records share it.
- `-` A step that reads at scale — a rebuild, a large unit — keeps a
  count of its causes, not all of them; the provenance still names every one.
- `-` The path is told by span links, which an observability backend has to
  show to be of use.
- `-` The change feed's format carries the trace context from its first
  version.
