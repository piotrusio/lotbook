# ADR-0015: Identity comes from the installation's IdP; authorization is Lotbook's

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

Lotbook answers over HTTPS on three interfaces: the ingest contract, used by
connectors (ADR-0014); the traversal API, used by systems and by people; and
the change feed, used by systems (ADR-0005). Every request has to say who
sends it, and Lotbook has to decide what that sender may do.

Every installation sits in a company that already runs an identity provider —
one vendor's here, another's there, a self-hosted one elsewhere. That IdP is
where the company keeps its people and its service accounts, rotates their
secrets, revokes them and audits their logins. The providers agree on how a
token is issued and verified — OpenID Connect and OAuth 2.0 — and disagree
on how they spell roles and groups inside it.

Weighed and rejected:

- **Credentials issued and stored by Lotbook.** One more store of secrets in a
  regulated company, with rotation, revocation and audit ours to build — all
  of which the IdP already does.
- **An integration per identity provider.** Code per vendor, for what the
  standard already covers.
- **Authorization read from the token's claims.** Roles and groups are named
  and nested differently by every provider; Lotbook's rules would follow the
  vendor.
- **An identity broker installed with Lotbook** in front of the company's
  IdP. A further stateful system with its own database, in every
  installation, for what the standard covers without it.
- **Client certificates.** The ingress in front of Lotbook has to hand the
  certificate through, and the ingress is the installation's, not ours.

## Decision

**Lotbook verifies bearer tokens issued by the installation's IdP, over OpenID
Connect and OAuth 2.0, and decides itself what their holder may do.**

- **The installation configures the issuers it trusts** — one, or several. For
  each, Lotbook reads its discovery document and its signing keys, and
  verifies a token's signature, issuer, audience — Lotbook — and expiry
  itself.
- **An identity is the pair issuer and subject.** Nothing else in the token is
  relied on.
- **People sign in through the IdP; systems use its service accounts**, with
  client credentials or with workload identity federation, which leaves no
  secret to store at all.
- **One service account per connector, never shared**, so the journal names
  which connector delivered each record (ADR-0014) and one connector can be cut
  off alone.
- **Authorization is held by Lotbook:** for each identity, the sources it may
  write, and its roles on the read side — reading the genealogy, reading the
  change feed, administering. Changes to it are recorded.
- **Authorization opens an interface, never a part of the genealogy.** A
  traversal with nodes cut out of it gives a false answer to a recall; what
  is reachable is the whole graph or nothing.

The SQL read schema is reached with PostgreSQL's own roles, and the life-cycle
commands with the cluster's; neither goes through this decision.

## Consequences

- `+` Lotbook stores no secret of its clients; rotation, revocation and login
  audit stay with the IdP the company already runs.
- `+` One code path for every provider; a self-hosted IdP and a vendor's are
  the same to Lotbook.
- `+` Lotbook's rules do not change with how a provider names roles.
- `-` Every installation registers Lotbook in its IdP, so that tokens can be
  issued for Lotbook's audience.
- `-` The mapping from identities to sources and roles is Lotbook's to keep
  and to administer.
- `-` Token verification and the refresh of signing keys are ours to write
  and test, with the standard library (ADR-0007).
- open: whether document data in a traversal's result is ever restricted is
  decided when the read side is designed.
