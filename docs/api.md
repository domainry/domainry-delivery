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
- `product.engineering.prepare.start`,
  `product.engineering.prepare.complete`
- `product.engineering.foundation.started`,
  `product.engineering.foundation.completed`,
  `product.engineering.foundation.failed`
- `feature.discovery.open`, `feature.discovery.replace`, `feature.delete`, `feature.confirm`, `feature.rebase`
- `feature.delivery.start` through the DeliveryRun creation endpoint

Engineering preparation starts only after at least one Feature revision is
confirmed and freezes those exact revisions as the initial Foundation scope.
One full-stack Engineer establishes the visual direction, Shell, routes, typed
HTTP contract, initial Model, capability owners, and module/SaaS deployment
choices, then submits them together with `product.engineering.prepare.complete`.
The bounded, content-addressed result opens the one-time Foundation installation
to the system. A failed installation either retries the same immutable input or
returns to the same Engineer preparation loop, so an engineering error never
becomes a user-owned recovery state.

Feature confirmation freezes the complete discovery and specification as one
immutable FeatureRevision against its exact ProductRevision baseline. Starting
a run rejects stale baselines rather than silently rebasing the requirement.
Only a draft Feature can be deleted. Deletion removes it from the active Product
aggregate while retaining its archived conversation messages and original
attachments for source traceability.

## DeliveryRun

```text
GET  /api/v1/workspaces/{workspace_id}/delivery-runs?product_id={product_id}
GET  /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}
GET  /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}/agent-context
POST /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}/commands
```

Agent commands:

- `delivery_unit.implementation.complete`
- `product_revision.record`
- `quality.record`
- `acceptance.bug.triage`
- `release_checks.replace`, `release_check.record`

Trusted system commands:

- `delivery_unit.verify`
- `delivery_unit.gap.report`
- `acceptance.environment.ready`
- `acceptance.bug.fix.ready`
- `release.deploy_result`, `release.reissue`

Human commands:

- `acceptance.bug.report`, `acceptance.bug.resolve`, `acceptance.bug.reopen`
- `acceptance.confirm`
- `release.prepare`, `release.approve`, `release.reconcile`

The server projection is authoritative for `available_actions` and
`release_gates`. Clients must reread it immediately before a mutation and must
not reproduce the state machine.

Delivery creates one implementation item and one trusted verification item for
each DeliveryUnit. The full-stack Engineer chooses the internal work order from
the confirmed Journey, current Product source, installed capabilities, and the
latest diagnostics. Delivery records the coarse authority boundary and evidence;
it does not persist interaction/model/frontend/backend substages.

`delivery_unit.gap.report` preserves the failed verification evidence and
returns the same Journey to its full-stack Engineer. It does not create a
frontend/backend owner state. The Engineer decides the repair order from the
diagnostics and current source, then resubmits the integrated revision.

The two DeliveryUnit boundaries are intentionally coarse: adaptive full-stack
implementation and trusted integrated verification. Model checks, contract
checks, and real Runtime Journeys run inside verification; they are not
separate Agent phases.

`delivery_unit.gap.report` preserves unaffected completed work and its
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
  "type": "delivery_unit.implementation.complete",
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
