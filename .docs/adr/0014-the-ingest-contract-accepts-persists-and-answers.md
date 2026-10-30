# ADR-0014: The ingest contract is one endpoint that accepts, persists and answers

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

Dozens to hundreds of source systems send their records to Lotbook through
connectors (ADR-0001). The ingest contract is the one inbound interface, public
and versioned from its first release.

`ingest` has one job: take what is sent, make it durable in the journal
(ADR-0009) and say so. What the records mean, in which order they are taken,
whether one arrived twice and whether a unit is complete are decided later, by
the workers, from the positions the records carry (ADR-0013). None of it is
asked of the connector or checked at the door.

Weighed and rejected:

- **An endpoint per source, or per kind of record.** It repeats in the address
  what the record already says, and a new source would need a change in
  Lotbook to be sent at all.
- **Rules on how a connector sends** — one request at a time, sources kept
  apart, records in order. They would make the contract depend on the
  connector's behaviour for what the workers can work out from the records
  themselves.

## Decision

**One endpoint, `POST /v1/source-events`, over HTTPS.** The body is JSON,
and may be sent gzip-compressed.

- **A request carries a list of records** in the envelope of ADR-0013 —
  source, position, unit with its last record marked, kind, content. A
  position is a non-negative integer of any length, written as text. Every
  value carries its type and is written as the canonical text of that type,
  so that precision, scale and zone reach the journal as the source held
  them. A request may hold records of any sources its sender may write to,
  in any order and in any number the endpoint accepts.
- **`ingest` checks two things:** that every record has the contract's shape,
  and that the sender may write to every record's source (ADR-0015). Then it
  writes the records to the journal.
- **Every record is kept with the identity that sent it** (ADR-0015), the
  request's trace context (ADR-0008) **and the version of the contract it
  was accepted under**, so that an audit sees not only what arrived and
  from whom, but by which rules it was taken in.
- **The contract is a machine-readable artifact** — OpenAPI with JSON Schema,
  its examples of requests accepted and refused included — that connectors
  validate against on their side and Lotbook's tests run on ours. A version,
  once released, is never edited.
- **The answers:**
  - `202` — the request's records are durable in the journal;
  - `400` — a record does not have the contract's shape; nothing is written;
  - `401` — no valid identity;
  - `403` — a record names a source its sender may not write; nothing is
    written;
  - `503` — the journal did not take the records; send the request again.
- **The version is in the path.** A new version is a new path, served beside
  the old one for as long as connectors use it.

## Consequences

- `+` Every connector sends to the same address in the same shape; a new
  source needs an authorization entry (ADR-0015) and no change to Lotbook.
- `+` `ingest` holds no state and scales by replicas.
- `+` A connector is never told how to send; whatever it does, the journal
  keeps it.
- `+` The contract's examples are the evidence that `ingest` takes in exactly
  what the contract says: every accepted example is accepted, every refused
  one refused.
- `-` A request sent again after a `503` or a lost answer can bring its
  records to the journal twice; the workers drop what they have already
  taken by its position (ADR-0013).
- `-` One malformed record refuses the whole request it came in.
- `-` Every value carries its type, and the request is larger for it;
  compression takes most of it back.
