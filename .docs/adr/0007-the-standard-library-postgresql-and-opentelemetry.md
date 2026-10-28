# ADR-0007: Lotbook imports the standard library, a PostgreSQL driver and OpenTelemetry

- **Status:** Accepted
- **Date:** 2026-10-28
- **Supersedes:** —
- **Superseded-by:** —
- **Issue/PR:** —

## Context

Lotbook is installed by others, into their clusters, for regulated work. Every
module it links is code its operators run without having chosen it: one more
thing to audit, to follow for vulnerabilities and to upgrade, and one more
author whose decisions reach the binary. Its graph of modules grows with each
one, mostly through dependencies nobody asked for.

Most of what a Go service usually imports, the standard library already does:
routing by method and path, structured logging, JSON, TLS, HMAC, embedded
files, memory-mapping a file, testing. What it does not do is speak the
PostgreSQL protocol (ADR-0009) or produce traces in a form an installation's
observability stack can take in — and tracing is not optional for a pipeline
whose every step happens after the request that caused it has been answered
(ADR-0008).

OpenTelemetry does not come alone. Its OTLP exporters are built on the
protocol's generated types, which bring a protobuf and gRPC stack and a few
`golang.org/x` modules into the binary — over HTTP as much as over gRPC. Only
the imports Lotbook writes itself are Lotbook's to choose; the tree beneath
OpenTelemetry comes with it or not at all.

Weighed and rejected:

- **The usual convenience modules** — an HTTP router, a UUID package, an
  assertion library, a migration library. Each saves a few dozen lines that
  the standard library makes short; the price is a dependency for the life
  of the project.
- **Contributed and third-party instrumentation** for HTTP and for the
  PostgreSQL driver. It is the widest part of the OpenTelemetry ecosystem, and
  what it instruments are two chokepoints Lotbook owns anyway.

## Decision

**Lotbook's code imports the Go standard library, one PostgreSQL driver and
OpenTelemetry — and nothing else.** The modules OpenTelemetry depends on are
accepted with it, as a whole, and are not imported directly.

- **OpenTelemetry is taken as its API, its SDK and its OTLP exporter over
  HTTP.** Nothing from its contrib repository and no third-party bridge.
  HTTP rather than gRPC for operational reasons only: plain HTTP passes the
  proxies and ingresses an installation already has. In dependencies the two
  cost the same.
- **Instrumentation is written here, at the chokepoints:** the HTTP server's
  middleware and the PostgreSQL driver's own tracing hooks.
- **Logs are structured, written to standard output, and carry the trace and
  span ids** of the context they are written in, so a log line is joined to
  its trace without a log pipeline of its own. OpenTelemetry's log API and SDK
  are stable, its OTLP log exporter is not yet (v0.x); logs go through
  OpenTelemetry once the exporter reaches v1.
- **The rule is enforced by the linter**, which refuses any import outside the
  allowed set. **Adding a dependency is a new ADR**, and the lint
  configuration changes with it. The whole tree — transitive modules
  included — is checked for known vulnerabilities by the same gate.

Development tools — the linter, the git hooks, the vulnerability scanner —
are pinned as tools and are not linked into the binary; they are outside this
rule.

## Consequences

- `+` What Lotbook imports can be read in one sitting, and every addition to
  it is a recorded decision.
- `+` Traces and logs reach whatever an installation runs, through one open
  protocol, without Lotbook choosing its backend.
- `-` The binary links some twenty modules, nearly all of them beneath
  OpenTelemetry's exporter. They are audited and upgraded as part of it, and
  a vulnerability in one of them can force an upgrade Lotbook did not plan.
- `-` Routing helpers, identifiers, migrations and test assertions are ours to
  write and test.
- `-` HTTP and database instrumentation is ours to keep in step with
  OpenTelemetry's semantic conventions.
- open: metrics — through OpenTelemetry's metric signal over the same
  exporter, or exposed for scraping.
