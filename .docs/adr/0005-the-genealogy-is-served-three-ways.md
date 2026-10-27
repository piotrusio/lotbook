# ADR-0005: The genealogy is served three ways, from one sequence

- **Status:** Accepted
- **Date:** 2026-10-27
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

A lot genealogy is not a query run when something has gone wrong. Kept in
real time, it is a data structure used across a company for analyses, by many
consumers, and joined with their own data — quality results, batch records,
costs, suppliers. Some consumers want answers to traversals, some want a copy
of their own, some want SQL.

The movement a graph node represents is a small part of the data the
documents behind it hold; consumers join the skeleton to those documents.

## Decision

**The genealogy is served three ways, all fed from the genealogy events in
sequence order (ADR-0004), each stating the sequence it is current to:**

1. **A traversal API** over HTTPS — upstream, downstream, impact — answered
   from the graph in memory (ADR-0006), with document data joined for the
   result only.
2. **A change feed** over HTTPS that a consumer reads from the sequence it
   last saw. A consumer that wants the changes in its own broker gets an
   outbound connector that reads the feed and publishes.
3. **SQL** through a public, versioned read schema of views. Internal tables
   may change; the views are a contract like the API. Analytical consumers
   read from a database replica, not from the primary.

## Consequences

- `+` Every consumer gets the genealogy in the form it works in, and all of
  them agree, because they follow one sequence.
- `+` Heavy analytical reads do not slow down accepting data.
- `-` Three public interfaces to version and keep stable instead of one.
- `-` The read schema constrains how the tables beneath it can change.
