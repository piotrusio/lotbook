# ADR-0004: Genealogy events are the source of truth; the graph is derived

- **Status:** Proposed
- **Date:** 2026-10-27
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

What reaches Lotbook is what a source system booked, in its own codes: one
movement type can mean a goods receipt or a production output depending on
the document around it, and a transformation of raw material into product is
two documents — an issue to an order and a receipt from it — that arrive
separately and in any order. Interpretation turns those bookings into what
happened to a lot, and its rules improve over time.

An auditor's question is not only *what is connected* but *why*: which
bookings, read by which rule, produced a link.

## Decision

**Lotbook keeps three levels, and only the first two are records.**

1. **The journal** — raw records as the source sent them, immutable.
2. **Genealogy events** — what interpretation concluded: a node added, an
   edge added, an edge invalidated. Immutable, numbered by one sequence, and
   each carries its provenance: the journal records it was read from and the
   version of the rule that read them. A reversal in the source is a new event
   that invalidates, never a deletion.
3. **The graph** — the current genealogy, derived from the events in sequence
   order. It can be thrown away and rebuilt.

Every way the genealogy is served (ADR-0005) is fed from the events, in
sequence order, and states the sequence it is current to.

## Consequences

- `+` *Why is lot A linked to lot B* is answered by provenance, not
  reconstructed.
- `+` Improved rules can be re-run over the journal; history is kept.
- `+` How the graph is stored is not a decision for ever: a new projection is
  built from the same events.
- `-` Storage grows with the journal and the events; nothing is updated in
  place.
- open: the node model — whether a movement, a lot and an order are all nodes
  — and how a transformation is correlated from its documents.
