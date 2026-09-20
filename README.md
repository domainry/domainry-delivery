# Domainry Delivery

Domainry Delivery is the lifecycle authority for Product and Feature. It versions how a product is explained and how it executes, then connects one complete PM requirement to implementation, verification, acceptance, and a real deployment receipt.

## Repository boundary

- `domainry-agent` owns durable conversations, model/tool runs, interactive confirmation, artifacts, and source access control.
- `domainry-delivery` owns Product, immutable ProductRevision, FeatureRevision, and DeliveryRun lifecycle state.
- `domainry-deck` embeds the local Agent Runtime and execution engine, then operates authoritative product state through a Delivery Binding.
- PM, RD, QA, and OP are local runtime roles. Delivery does not depend on remote Agent services.

`ProductStory` is the human narrative. `ProductDefinition` is the machine-executable page, object, field, state, rule, action, and workflow schema. Both are stored in the same immutable ProductRevision. One Feature represents one complete requirement. PM turns refine one mutable Feature discovery workspace that retains stated evidence, inferences, corrections, conflicts, assumptions, options, decisions, the derived specification, and exact source lineage. Delivery ranks the next question and blocks confirmation while material uncertainty remains. Human confirmation freezes that workspace into an immutable FeatureRevision; conversational turns never create versions by themselves.

## Implemented contract

- Product create/list/detail/command/Agent Context APIs.
- Atomic ProductStory and ProductDefinition versioning.
- Evidence-led Feature discovery workspace, Delivery-ranked next question, human confirmation, DeliveryRun start, and installation state machine.
- One exact FeatureRevision per DeliveryRun.
- Optional WorkItems with dependency-cycle validation.
- TestCases derived from Feature acceptance criteria, with Issue fix/retest closure.
- Environment-bound ReleaseChecks and real build, test, release, and deployment evidence.
- Atomic Feature installation after a live Release with a deployment receipt.
- SQLite persistence with transactional optimistic concurrency and `client_id` idempotency.
- Server-owned `available_actions` and `release_gates` projections.
- Server-resolved Identity that overwrites command actors and enforces Workspace permissions.
- One public SDK contract implemented by both in-process Module and remote SaaS bindings.
- Locale metadata and localized Delivery-owned release gates and error messages for English, Japanese, Korean, Spanish, Portuguese, French, German, Italian, and Turkish.

## Run the SaaS service

```bash
go run ./cmd/domainry-delivery
```

Environment:

- `DELIVERY_ADDR`: listener address; defaults to `127.0.0.1:8096`.
- `DELIVERY_DB`: SQLite path; defaults to `data/domainry-delivery.db`.
- Identity remote binding configuration required by `domainry-identity-sdk`.

Production startup never inserts demo data. The service exposes unauthenticated health and descriptor endpoints; every Workspace route requires an authenticated Identity principal.

## Verify

```bash
go test ./...
go vet ./...
```

See [HTTP API](docs/api.md), [Agent integration](docs/agent-integration.md), and [delivery plan](TODO.md).
