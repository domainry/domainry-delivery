# HTTP API v1

All Workspace routes require `Authorization: Bearer ...`. Identity middleware
resolves the principal and Workspace; command JSON cannot declare an actor.
In SaaS mode, the embedded external Identity Bridge validates that bearer token
with Verdent Passport. `GET /auth/external/session` provisions (on first
access) and returns the verified user's personal Workspace. Deck uses that
server-issued `workspace_id` for subsequent Workspace routes; neither Deck nor
request JSON may choose it.

The public bridge discovery endpoints are `GET /auth/external/config` and
`GET /auth/external/client.js`. `GET /auth/external/session` is authenticated.
Delivery stores no Verdent password and issues no replacement browser session.
Errors contain a stable code, localized presentation message, and optional
details. Supported locales are `en`, `zh`, `zh-hant`, `ja`, `ko`, `es`, `pt`,
`fr`, `de`, `it`, `tr`, and `ar`.

All instant fields named `*_at` are integer UTC Unix milliseconds in both
requests and responses. `target_date` is a date-only business value and
remains a string. User interfaces format instants in the user's timezone.

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

The Feature archive is scoped under the same Product and Workspace:

```text
POST   /api/v1/workspaces/{workspace_id}/products/{product_id}/features/{feature_id}/messages
GET    /api/v1/workspaces/{workspace_id}/products/{product_id}/features/{feature_id}/messages?cursor={message_id}&limit=100
POST   /api/v1/workspaces/{workspace_id}/products/{product_id}/features/{feature_id}/attachments
GET    /api/v1/workspaces/{workspace_id}/products/{product_id}/features/{feature_id}/attachments
GET    /api/v1/workspaces/{workspace_id}/products/{product_id}/features/{feature_id}/attachments/{attachment_id}
DELETE /api/v1/workspaces/{workspace_id}/products/{product_id}/features/{feature_id}/attachments/{attachment_id}
POST   /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}/acceptance-attachments
GET    /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}/acceptance-attachments/{attachment_id}
DELETE /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}/acceptance-attachments/{attachment_id}
```

Message writes contain `client_id`, `device_id`, `expected_revision: 0`,
`turn_id`, `role` (`user` or `assistant`), original `text`, and
`attachment_ids`. Attachment uploads are JSON with `client_id`, `device_id`,
`expected_revision: 0`, `filename`, and base64 `data`; removal contains
`client_id`, `device_id`, and the attachment's `expected_revision`. Downloads
return attachment metadata plus base64 `data`. A removal hides the attachment
from the active list but keeps the original bytes readable for historical PRD
citations.

Acceptance screenshots use the same bounded immutable object-storage contract
but a DeliveryRun-scoped metadata namespace. A client uploads one PNG as
base64 before `acceptance.bug.report`, submits only the resulting
`attachment://{attachment_id}` reference, and QA downloads the image through
the authenticated DeliveryRun endpoint. Screenshot bytes never enter the
DeliveryRun JSON, acceptance event stream, or SQL BLOB columns.

Product commands come from the typed command catalog:

- `product.create`, `product.delete`, `product.restore`
- `product.engineering.frontend.start`,
  `product.engineering.frontend.complete`
- `product.engineering.foundation.started`,
  `product.engineering.foundation.completed`,
  `product.engineering.foundation.failed`
- `feature.discovery.open`, `feature.discovery.replace`, `feature.confirm`, `feature.rebase`
- `feature.delivery.start` through the DeliveryRun creation endpoint

Frontend completion is a technical initialization gate backed by the recorded
artifact, design contract and entry points. A verified completion exposes
Foundation installation directly; user review happens later in the executable
acceptance environment.

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
- `delivery_unit.frontend.complete`
- `delivery_unit.backend.complete`
- `product_revision.record`
- `quality.record`
- `acceptance.bug.triage`
- `release_checks.replace`, `release_check.record`

Trusted system commands:

- `delivery_unit.model.verify`
- `delivery_unit.contract.verify`
- `delivery_unit.gap.report`
- `delivery_unit.journey.complete`
- `acceptance.environment.ready`
- `acceptance.bug.fix.ready`
- `release.deploy_result`

Human commands:

- `acceptance.bug.report`, `acceptance.bug.resolve`, `acceptance.bug.reopen`
- `acceptance.confirm`
- `release.prepare`, `release.approve`, `release.reconcile`

The server projection is authoritative for `available_actions` and
`release_gates`. Clients must reread it immediately before a mutation and must
not reproduce the state machine.

Delivery creates exactly seven fixed development items when a DeliveryUnit is
created: interaction modeling, domain modeling, model verification, frontend
implementation, backend implementation, contract verification, and the real
Runtime journey. Accepting a phase command completes that fixed item and starts
the next one; the agent does not initialize or subdivide this sequence.

`delivery_unit.gap.report` preserves unaffected completed phases and their
evidence. Delivery derives stable repair identities from the routed diagnostic
owner, code, category, and path, then creates or reopens only internal repair
items in the owning phase. A repeated failure for the same diagnostic reuses
the existing repair item. Only the owning phase and affected downstream gates
are revalidated; the complete development sequence is not replayed.

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
