# .docs/CLAUDE.md

Rules for writing anything under `.docs/`. The documents are the reference; this file says where a thing belongs and how it is written.

## Where a decision belongs

| The thing | Goes to |
|---|---|
| Why we chose a shape, with the alternatives and the trade-offs | an ADR in [`adr/`](adr/) |
| How a part of the system works as a whole | the owning [`system/<subject>.md`](system/) |
| What a lot, a movement or an interpretation rule is, and what it refuses | **the code** — the doc comment above the type and its test. Never a document here |
| A defect, or work in flight | an issue on GitHub — a few lines, never a specification |
| A rule an agent must follow | the nearest `CLAUDE.md` — not a doc |

System documents evolve, ADRs accumulate. Reading a system document and wondering *why is it this way* — the answer is in `adr/`.

**Docs ship with the change they describe.** A doc that contradicts the repository is read as true and is worse than no doc: when a described thing changes or is removed, its description changes with it, in a `docs` commit of the same body of work. Git history is the archive — delete a wrong or replaced document rather than keeping an archive directory.

**This repository is public.** Documents here state technical decisions and technical reasons only. No customer names, no customer data, no figures out of a customer's system. No business model, pricing, commercial plans or sales arguments, and no reasoning about support costs — those are not decided in this repository. A lesson learned at a customer is written as the lesson, not as the customer.

## ADRs

- **An ADR fixes the shape; the owning system document names the tools.** Write in solution classes; a product name belongs in an ADR only as a rejected alternative or as settled, load-bearing substrate (PostgreSQL, Kubernetes).
- **Supersede a decision; correct a description.** When the *decision* changes, a new ADR carrying `Supersedes: ADR-NNNN`, and the old one gets `Superseded-by`; when the text describes a standing decision wrongly, fix the text in place.
- **Delete an ADR that never governed anything.** `Deprecated` is for a decision that was in force and was withdrawn without replacement. Numbers are never reused; gaps stay.
- **One decision per file.**

To add one: copy [`adr/template.md`](adr/template.md) to `NNNN-<short-slug>.md` with the next free zero-padded number, keep `Status: Proposed` while it is being discussed, flip to `Accepted` once taken. There is no index to maintain — the directory listing and each file's metadata are the index.

## System documents

A system document describes a part of the system as a whole — what spans several modules and therefore sits in no single file. It links the ADRs that fixed its shape instead of repeating their arguments. While a part is designed but not built, its document says so in its first lines, and that line goes the day the code catches up.
