# ADR-0013: The journal holds what each source said, in its order; the genealogy events are the other record

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** ADR-0004
- **Superseded-by:** —
- **Issue/PR:** —

## Context

ADR-0004 took every source system to be relational and transactional, and
made the journal that system's transaction log on its tables. Many are: an
invoice, an issue to production or a goods receipt is assembled there from
several tables — a header, its lines, the batches on them, master data
around them — all of it changing row by row, committed in transactions.

Not every source is. A system built on events holds no tables to mirror and
commits no transactions across them; what it can say is the stream of its
events, in the order it emitted them. A contract that carries only changes to
rows has nothing to offer it but to be made to look like a database.

What Lotbook serves is the same either way: a genealogy of lots, and the
documents behind each step of it. Between the two sit the assembly of
documents and the interpretation of what each document means for a lot —
both specific to each source system, and both expected to be corrected and
improved over time.

An auditor's question is not only *what is connected* but *why*: which
source data, read by which rule, produced a link — down to what the source
itself said.

## Decision

**Two levels are records and are never changed; every other level is derived
from them and can be rebuilt.**

1. **The journal** *(record)* — what each source said, as the connector sent
   it, in the source's order.
   - **A source is one ordered stream.** A system with several independent
     streams is several sources.
   - **Every record carries its source, its position** — increasing within the
     source — **and the unit it belongs to:** the records that make sense only
     together, with the last of them marked. In a transactional source a unit
     is a transaction; in an evented one, each event is a unit of its own.
   - **A record is of a kind:** a **row change** — table, operation, the row's
     key and data — or an **event** — its type, the key of what it is about,
     its data and when it occurred in the source. A new kind is a new version
     of the ingest contract.
2. **The mirror** *(derived)* — the current state of a relational source's
   tables, maintained from its row changes. Assembling a document after one
   row changed needs the current state of the rows around it. A source of
   events has no mirror.
3. **The documents** *(derived)* — versioned, assembled only from complete
   units: from the mirror for row changes, from the events themselves for an
   evented source. Half a unit is an inconsistent document. How a document of
   a given type is assembled is specific to the source system.
4. **The genealogy events** *(record)* — what interpretation concluded from
   the documents: a node added, an edge added, an edge invalidated. Numbered
   by one sequence, and each carries its provenance: the documents and
   journal positions it was read from and the version of the rule that read
   them. A reversal in the source is a new event that invalidates, never a
   deletion.
5. **The graph** *(derived)* — the current genealogy, a skeleton of nodes,
   edges and document keys, built from the events in sequence order.

**Queries are answered from the graph and the documents together:** the
traversal runs on the graph, and the documents are fetched for the result.

## Consequences

- `+` A relational source and an evented one reach the same documents, the
  same interpretation and the same genealogy; neither is made to imitate the
  other.
- `+` A defect in assembly or in key derivation is repaired by rebuilding the
  mirror and the documents from the journal; the source data is never lost to
  our own bug.
- `+` A defect in interpretation is repaired by the corrected rule reading
  the journal again and writing new genealogy events — invalidating the wrong
  edges, adding the right ones, with the new rule version in their
  provenance. The wrong interpretation stays in the history: when an edge
  existed, and why it was withdrawn, remain answerable.
- `+` *Why is lot A linked to lot B* is answered by provenance, down to the
  rows the source committed or the events it emitted.
- `+` Connectors stay ignorant of documents and of lots: they carry what the
  source said.
- `+` How the mirror, the documents or the graph are stored is not a decision
  for ever; each is rebuilt from the level before it.
- `-` The source's data is held more than once — as the journal, the mirror
  and the documents. The journal is the one that cannot be dropped
  (ADR-0011).
- `-` Assembly definitions and interpretation rules are written per source
  system, and assembly differs by kind of record.
- open: the node model — which kinds of nodes and edges the graph has.
