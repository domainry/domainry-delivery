# Local Agents and Delivery

## Runtime relationship

Deck runs one local in-process Agent Runtime. PM, RD, QA, and OP are four role configurations within that runtime, not four remote services and not a cloud dependency.

Delivery is the lifecycle authority shared by those roles:

- PM reads the confirmed Product baseline, the historical Feature catalog, and the current Feature discovery workspace before interpreting each new message. It keeps user-stated evidence distinct from PM inference, reconciles corrections and conflicts, presents consequential options with a recommendation and tradeoffs, and submits the complete updated workspace with exact conversation sources.
- Delivery validates the workspace, ranks open questions by impact, dependency, uncertainty, and answer cost, and returns the one question PM must ask next. Multiple turns update the same workspace rather than creating fake requirement versions.
- PM does not design the implementation or create a ProductRevision. RD, QA, and OP own delivery after the Product owner confirms the Feature.
- An authenticated Product owner confirms a ready Feature, freezing an immutable FeatureRevision against the exact ProductRevision baseline. Confirmation does not silently rewrite the Product definition.
- RD starts a DeliveryRun for the exact confirmed FeatureRevision and decides whether WorkItems are useful.
- QA verifies the immutable build against generated TestCases and records real evidence; a failure creates an Issue.
- OP prepares environment-bound ReleaseChecks.
- A trusted deployment adapter records Artifact, Revision, Environment, outcome, and Receipt evidence.
- A successful deployment atomically installs the exact FeatureRevision into Product.

The same Delivery Binding contract is implemented by an in-process Module and a remote SaaS adapter. Capabilities, state transitions, identity semantics, idempotency, and optimistic concurrency remain identical.

## Source lineage

Delivery does not copy chat messages and does not own model run state. The mutable discovery workspace aggregates source references while PM and the user refine the requirement. The confirmed FeatureRevision then freezes:

- the exact `conversation_id` that formed the requirement;
- the exact `run_id` and optional `before_step` boundary;
- business `source_ids` supporting the facts;
- `decision_ids` that changed the executable definition.

The Agent Runtime owns the Conversation, Run, and source bodies. Delivery preserves immutable references and connects them to ProductRevision, FeatureRevision, DeliveryRun, Build, TestRun, Release, and deployment receipt.

## Authority boundary

- An Agent can propose a Feature and advance implementation, test, and release-preparation work only through currently available Agent actions.
- Feature confirmation, business acceptance, release preparation, release approval, and reconciliation require an authenticated human assigned to the required DeliveryRun role.
- Deployment outcome and successful installation require a trusted system credential.
- Identity is resolved server-side. A client cannot declare its actor kind, permissions, or Workspace authority.
