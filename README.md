# Domainry Delivery

Domainry Delivery is the owner of Product, immutable ProductRevision,
FeatureRevision, DeliveryRun, and the complete delivery lifecycle. It connects
one confirmed business requirement to implementation, Runtime verification,
QA, business acceptance, release, a real deployment receipt, and atomic
installation into the next ProductRevision.

## Ownership and module boundaries

- `domainry-delivery` backs up raw PM conversation messages, stores original
  Feature attachment bytes in a configured object store or persistent volume,
  and verifies the source references used by Feature revisions. It does not
  require a remote Agent service.
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
- `internal/infrastructure/persistence`: shared relational persistence built
  exclusively with `domainry-orm` schema/query builders, plus MySQL and SQLite
  adapters; repositories contain no handwritten SQL.
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
- `DELIVERY_ATTACHMENT_STORAGE_PATH`: absolute path for original attachment
  bytes on a persistent volume. Set this for the dev deployment. Either this
  setting or S3 storage is required at startup.
- `DELIVERY_ATTACHMENT_S3_REGION`, `DELIVERY_ATTACHMENT_S3_BUCKET`, and
  optional `DELIVERY_ATTACHMENT_S3_PREFIX`: alternative S3 object storage.
  Configure either the file path or S3, never both.
- `DOMAINRY_IDENTITY_BRIDGE_CONFIG_FILE`: strict external-provider bridge
  configuration, default `conf/identity-external.json`. The packaged
  configuration validates Verdent Passport access tokens and creates one
  personal Delivery Workspace per verified user. Delivery does not require a
  remote Identity service or service credential.
- `DELIVERY_DEV_IDENTITY`: optional local-only test identity, deliberately
  restricted to loopback listeners and never used by the dev deployment.
The dev Kubernetes configuration lives in the separate devops repository at
`domainry-delivery/k8s/dev`. It uses MySQL, environment-owned database and
the packaged Identity Bridge configuration, a dedicated attachment PVC,
Jenkins image builds, ECR, and Argo CD; no database values are committed.

All Delivery-owned Product, Feature, DeliveryRun, conversation and attachment
timestamp columns and API fields use UTC Unix milliseconds. Foundation-owned
operation metadata retains its separate storage contract and is not exposed
as Delivery API dates.
`target_date` is a calendar date rather than an instant, so it remains a
date-only value. Clients format timestamps in the user's local timezone.

## Verify

```bash
go test ./...
go vet ./...
```

See [HTTP API](docs/api.md), [Deck integration](docs/agent-integration.md), and
the evidence-backed [refactor checklist](TODO.md).
