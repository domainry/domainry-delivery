# HTTP API v1

All responses use JSON. Error responses contain a stable code, localized presentation message, and optional details:

```json
{"code":"revision_conflict","message":"The revision changed; read the resource again before submitting.","details":{"actual_revision":2}}
```

Every Workspace route requires `Authorization: Bearer ...`. Clients may send `Accept-Language`; unsupported languages fall back to English. Supported locales are `en`, `ja`, `ko`, `es`, `pt`, `fr`, `de`, `it`, and `tr`. Identity middleware resolves the principal and the application overwrites any actor value. In fact, actor is not part of the accepted command JSON.

## Discovery

```text
GET /healthz
GET /api/v1/delivery/descriptor
GET /api/v1/workspaces/{workspace_id}/session
```

The descriptor exposes protocol version, deployment mode, capabilities, and supported locales. Session exposes only non-secret authenticated identity facts required for human role assignment.

## Product

```text
GET  /api/v1/workspaces/{workspace_id}/products
GET  /api/v1/workspaces/{workspace_id}/products/{product_id}
GET  /api/v1/workspaces/{workspace_id}/products/{product_id}/agent-context
POST /api/v1/workspaces/{workspace_id}/products/{product_id}/commands
POST /api/v1/workspaces/{workspace_id}/products/{product_id}/delivery-runs/{delivery_run_id}
```

Product commands:

- `product.create`: creates Story, Definition, and Decisions atomically with `expected_revision: 0`.
- `feature.discovery.replace`: replaces one mutable Feature discovery workspace with evidence, scenarios, assumptions, conflicts, options, decisions, the derived specification, and exact conversation sources. Delivery validates the complete state and selects the highest-value next question.
- `feature.confirm`: authenticated human confirmation that freezes the current discovery workspace as an immutable FeatureRevision against its exact ProductRevision baseline.
- `feature.delivery.start`: local RD workload starts a DeliveryRun bound to the exact confirmed FeatureRevision.

Product detail returns server-owned `available_actions`. Agent Context includes only actions currently available to an Agent and explicitly lists human/system boundaries.

## DeliveryRun

```text
GET  /api/v1/workspaces/{workspace_id}/delivery-runs?product_id={product_id}
GET  /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}
GET  /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}/agent-context
POST /api/v1/workspaces/{workspace_id}/delivery-runs/{delivery_run_id}/commands
```

The complete projection includes `workflow.available_actions` and `workflow.release_gates`. Deck renders these fields and does not recreate lifecycle conditions.

Agent commands:

- `work.plan.replace`, `work.start`, `work.complete`
- `build.create`, `build.deploy_to_test`
- `test.record`
- `issue.start_fix`, `issue.complete_fix`
- `release_checks.replace`, `release_check.record`

Human commands:

- `acceptance.record`
- `release.prepare`, `release.approve`
- `release.reconcile`

Trusted system command:

- `release.deploy_result`

A successful `release.deploy_result` must include the trusted deployment
receipt and the absolute HTTP(S) `launch_url` of the deployed SaaS product.
After installation, Product projections expose that destination as
`product.current_deployment`; clients must not derive a URL from
`environment_ref`.

## Command envelope

```json
{
  "client_id": "0199...",
  "expected_revision": 7,
  "type": "work.start",
  "payload": {"work_item_id":"DEV-01"}
}
```

Every write requires a unique `client_id` and current `expected_revision`. Retrying the same command with the same client ID returns its original receipt. Reusing that ID for a different command fails. Revision conflicts return HTTP 409 and the actual revision.
