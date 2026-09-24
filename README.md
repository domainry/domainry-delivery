# Domainry Delivery

Domainry Delivery is the owner of Product, immutable ProductRevision,
FeatureRevision, DeliveryRun, and the complete delivery lifecycle. It connects
one confirmed business requirement to implementation, Runtime verification,
QA, business acceptance, release, a real deployment receipt, and atomic
installation into the next ProductRevision.

## Ownership and module boundaries

- `domainry-agent` owns conversations, runs, source publication, artifacts,
  and interactive confirmation. Delivery stores the immutable references
  supplied with a command, never message bodies or attachment bytes, and does
  not require Agent to be online.
- `domainry-delivery` owns Product, Feature, revision, lifecycle and release
  state. Every transition is validated by the domain layer.
- `domainry-deck` runs the local PM/RD/QA/OP roles and submits typed Rust
  requests that mirror the independently published Delivery SDK contract.
  Client code never writes a status directly.

The implementation is physically split by responsibility:

- `internal/domain/product`: Product, Feature and immutable revision rules.
- `internal/domain/deliveryrun`: the single DeliveryUnit lifecycle.
- `internal/domain/lifecycle`: cross-aggregate start/install transitions.
- `internal/application`: use cases and narrow repository ports.
- `internal/infrastructure/persistence`: shared relational persistence plus
  MySQL and SQLite adapters.
- `internal/assembly/module` and `internal/assembly/saas`: separate composition
  roots using the same domain, application and schema.
- `internal/transport/http`: authenticated SaaS transport only.

There is no catch-all `internal/domain/delivery` package, legacy
WorkItem/Build/TestRun state machine, attachment BLOB store, or aggregate
`state_json` persistence.

## Lifecycle contract

`ProductStory` and executable `ProductDefinition` are committed together in
one immutable ProductRevision. A confirmed FeatureRevision freezes its exact
Product baseline and complete Conversation/Run/source/decision lineage. A
DeliveryRun can start only from that baseline.

Each DeliveryUnit records typed evidence for interaction, model, backend,
frontend, contract, gap and journey checks. Model, Runtime, contract and
journey verification is accepted only from a trusted system principal. The
candidate ProductRevision, QA, acceptance, release checks, deployment receipt
and installation must all bind to the same clean Git revision. Installation
appends the immutable ProductRevision and advances Product and Feature state in
one database transaction.

All writes include `client_id` and `expected_revision`. Foundation
`_operations` stores the canonical fingerprint and original terminal receipt,
so exact retries replay the first result and changed facts conflict.

## Run the SaaS service

```bash
go run ./cmd/domainry-delivery
```

Environment:

- `DELIVERY_ADDR`: listener address, default `127.0.0.1:8096`.
- `DELIVERY_RUNTIME_ID`: descriptor/runtime audience.
- `DELIVERY_DB_DRIVER`: `sqlite` for local development or `mysql`.
- `DELIVERY_DB`: SQLite path when the driver is `sqlite`.
- `DELIVERY_MYSQL_DSN`: Go MySQL driver DSN when the driver is `mysql`.
- `DOMAINRY_IDENTITY_BRIDGE_CONFIG_FILE`: strict external-provider bridge
  configuration, default `config/identity-external.json`. The packaged
  configuration validates Verdent Passport access tokens and creates one
  personal Delivery Workspace per verified user. Delivery does not require a
  remote Identity service or service credential.
- `DELIVERY_DEV_IDENTITY`: optional local-only test identity, deliberately
  restricted to loopback listeners and never used by the dev deployment.
The dev Kubernetes configuration lives in the separate devops repository at
`domainry-delivery/k8s/dev`. It uses MySQL, environment-owned database and
the packaged Identity Bridge configuration, Jenkins image builds, ECR, and
Argo CD; no database values are committed.

## Verify

```bash
go test ./...
go vet ./...
```

See [HTTP API](docs/api.md), [Agent integration](docs/agent-integration.md), and
the evidence-backed [refactor checklist](TODO.md).
