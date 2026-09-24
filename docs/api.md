# HTTP API v1

All Workspace routes require `Authorization: Bearer ...`. Identity middleware
resolves the principal and Workspace; command JSON cannot declare an actor.
Errors contain a stable code, localized presentation message, and optional
details. Supported locales are `en`, `ja`, `ko`, `es`, `pt`, `fr`, `de`, `it`,
and `tr`.

## Discovery

```text
GET /healthz
GET /api/v1/delivery/descriptor
GET /api/v1/workspaces/{workspace_id}/session
```

## Product and Feature

```text
GET  /api/v1/workspaces/{workspace_id}/products
GET  /api/v1/workspaces/{workspace_id}/products/{product_id}
GET  /api/v1/workspaces/{workspace_id}/products/{product_id}/agent-context
POST /api/v1/workspaces/{workspace_id}/products/{product_id}/commands
POST /api/v1/workspaces/{workspace_id}/products/{product_id}/delivery-runs/{delivery_run_id}
```

Product commands come from the typed command catalog:

- `product.create`, `product.delete`
- `product.engineering.frontend.start`,
  `product.engineering.frontend.complete`
- `product.engineering.foundation.started`,
  `product.engineering.foundation.completed`,
  `product.engineering.foundation.failed`
- `feature.discovery.open`, `feature.discovery.replace`, `feature.confirm`
- `feature.delivery.start` through the DeliveryRun creation endpoint

Feature confirmation freezes the complete discovery and specification as one
immutable FeatureRevision against its exact ProductRevision baseline. Starting
a run rejects stale baselines rather than silently rebasing the requirement.

## DeliveryRun

```text
GET  /api/v1/workspaces/{workspace_id}/delivery-runs?product_id={product_id}
GET  /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}
GET  /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}/agent-context
POST /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}/commands
```

Agent commands:

- `delivery_unit.interaction.complete`
- `delivery_unit.model.complete`
- `delivery_unit.backend.complete`
- `delivery_unit.frontend.complete`
- `product_revision.record`
- `quality.record`
- `release_checks.replace`, `release_check.record`

Trusted system commands:

- `delivery_unit.model.verify`
- `delivery_unit.contract.verify`
- `delivery_unit.gap.report`
- `delivery_unit.journey.complete`
- `release.deploy_result`

Human commands:

- `acceptance.confirm`
- `release.prepare`, `release.approve`, `release.reconcile`

The server projection is authoritative for `available_actions` and
`release_gates`. Clients must reread it immediately before a mutation and must
not reproduce the state machine.

## Command envelope and replay

```json
{
  "client_id": "0199...",
  "expected_revision": 7,
  "type": "delivery_unit.backend.complete",
  "payload": {}
}
```

Every write requires `client_id` and `expected_revision`. The command
fingerprint includes the authenticated actor, command, canonical payload,
Workspace and resource identities from the URL. An exact retry returns the
original terminal receipt. Reusing the ID with any changed fact fails;
optimistic revision conflicts return HTTP 409 with the actual revision.

A successful deployment result includes the trusted provider receipt, exact
environment and an absolute HTTP(S) `launch_url`. Installation is part of the
same transaction that commits the live run and new ProductRevision.
