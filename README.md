<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/assets/lotbook-logo-dark.svg">
    <img src=".github/assets/lotbook-logo-light.svg" alt="lotbook." width="360">
  </picture>
</p>

<p align="center"><strong>Every lot has a history.</strong></p>

Lotbook is an open-source lot genealogy engine. It reads events from your ERP, WMS and MES, understands what each operation means for a lot, and keeps the full genealogy of every lot as a graph. Trace up to every source or down to every customer.

> **Status:** early development. There is nothing to install yet. Watch the repository to follow progress.

## How it works

1. **Ingest.** Connectors read goods receipts, production orders, transfers and deliveries from ERP, WMS and MES. Your systems keep running as they are.
2. **Understand.** The domain model translates each booking into what happened to the lot: received, moved, transformed, packed, released or shipped.
3. **Trace.** Lots and their links form a graph. Follow any lot upstream or downstream, with the full event history at every step.

## Why

The same booking code can mean different things. In SAP, movement type 101 is both a goods receipt for a purchase order and the output of a production order. Lotbook reads the context of every operation and records what really happened to the lot, so a recall, a quality investigation or an audit gets a complete answer in both directions.

## Design goals

- **Open standard.** Events follow GS1 EPCIS 2.0, so genealogy can be exported and shared across companies.
- **Self-hosted.** Your lot data stays on your infrastructure.
- **Built for regulated work.** A complete audit trail on every event.

## Contributing

Ideas and [issues](https://github.com/piotrusio/lotbook/issues/new/choose) are welcome, especially from people who know how lot traceability works in their ERP, WMS or MES. We don't accept pull requests from outside contributors. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Licensed under the [Apache License 2.0](LICENSE).
