# ADR-0004: The journal and the genealogy events are the records; everything else is derived

- **Status:** Superseded
- **Date:** 2026-10-27
- **Supersedes:** —
- **Superseded-by:** ADR-0013
- **Issue/PR:** —

## Context

The source systems — ERP, WMS, MES — are relational and transactional. They
hold no documents: an invoice, an issue to production or a goods receipt is
assembled from several tables — a header, its lines, the batches on them,
master data around them — and all of it changes row by row, committed in
transactions. What a connector can faithfully carry out of such a system is
its changes to rows, in the order they were committed.

What Lotbook serves is the opposite end: a genealogy of lots, and the
documents behind each step of it. Between the two sit the assembly of
documents from rows and the interpretation of what each document means for a
lot — both specific to each source system, and both expected to be corrected
and improved over time.

An auditor's question is not only *what is connected* but *why*: which
source data, read by which rule, produced a link.

## Decision

**Two levels are records and are never changed; every other level is derived
from them and can be rebuilt.**

1. **The journal** *(record)* — the source's transaction log on its tables, as
   the connector sent it: per change the source, the table, the operation, the
   row's key and data, the source transaction it belongs to and its commit
   position. Kept in commit order, with transaction boundaries. The ingest
   contract carries exactly this — changes to rows, not documents.
2. **The mirror** *(derived)* — the current state of each source's tables,
   one schema per source, maintained from the journal. Assembling a document
   after one row changed needs the current state of the rows around it.
3. **The documents** *(derived)* — documents assembled from the mirror,
   versioned. A document is assembled only from complete source
   transactions; half a transaction is an inconsistent document. How a
   document of a given type is assembled is specific to the source system.
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

- `+` A defect in assembly or in key derivation is repaired by rebuilding the
  mirror and the documents from the journal; the source data is never lost to
  our own bug.
- `+` A defect in interpretation is repaired by the corrected rule reading
  the journal again and writing new genealogy events — invalidating the wrong
  edges, adding the right ones, with the new rule version in their
  provenance. The wrong interpretation stays in the history: when an edge
  existed, and why it was withdrawn, remain answerable.
- `+` *Why is lot A linked to lot B* is answered by provenance, down to the
  rows the source committed.
- `+` Connectors stay ignorant of documents and of lots: they read changes.
- `+` How the mirror, the documents or the graph are stored is not a decision
  for ever; each is rebuilt from the level before it.
- `-` The source's data is held more than once — as the journal, the mirror
  and the documents. The journal is the one that cannot be dropped; its old
  partitions can be moved to cheaper storage.
- `-` Assembly definitions and interpretation rules are written per source
  system.
- open: the node model — which kinds of nodes and edges the graph has.
