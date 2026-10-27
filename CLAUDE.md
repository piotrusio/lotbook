# CLAUDE.md

Lotbook is an open-source lot genealogy engine: one Go service that reads raw bookings from ERP, WMS and MES connectors, interprets what each one means for a lot, and keeps the genealogy of every lot as a graph. Connectors live in their own repositories; this one holds the service and the ingest contract they talk to.

## Test first

Write the test before the implementation, **run it, and see it fail.** A test nobody watched fail proves nothing about itself.

Work on Go code is delivered as commits in this order:

| # | Type | What is in it | The state it leaves |
|---|---|---|---|
| 1 | `test` | the tests, and nothing else — only `*_test.go` | **red**, and run to prove it |
| 2 | `feat` / `fix` / `refactor` | the implementation | **green** |
| 3 | `docs` | an ADR, **when there is one** | — |

## What "done" means

| Touched | Must pass |
|---|---|
| any Go code, `go.mod`, `.golangci.yml` | `make lint test` |
| before a PR | `make check` — everything CI runs |

The one exception lasts one turn: at the `test` commit the tests are red on purpose, and the bar is `make lint-impl` green while `make test` genuinely fails.

**The gates run themselves.** Session hooks (`.claude/settings.json` → `.scripts/hooks/`) gofmt-check every edited Go file and run the gate when the turn ends; a red gate comes back into the turn. Git hooks (`.lefthook.yml`) run gofmt and the lint on commit. A session without the hooks owes the same gates by hand.

## Conventions

- **Commits and PR titles:** `type(scope): subject`, scope optional, validated by `.scripts/validate-commit-msg.sh`. Types: `feat | fix | refactor | test | build | ci | docs | chore`. PRs are squash-merged, so the PR title is the commit on `main`.
- **Go version:** the `go` line in `go.mod` is the minimum patch release with no known vulnerabilities. Raise it when a Go security release lands; `make vuln` fails otherwise.
- **Tooling is pinned in one place:** golangci-lint in the `Makefile`, lefthook and govulncheck as `tool` lines in `go.mod`. CI runs `make check` and states nothing of its own.
- **Commands:** `make help`.
