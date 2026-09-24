# Agent, Deck and Delivery integration

Deck executes PM, RD, QA and OP as local roles in its embedded runtime. Those
roles propose typed Delivery commands; they are not independent services and a
prompt never grants authority. Rust orchestration rereads the current Delivery
projection, checks `available_actions` and revision, then submits through the
Delivery SDK. TypeScript is presentation-only.

## Source ownership

Agent owns Conversation, Run, source access, decision provenance and artifact
bytes. Delivery owns only immutable lineage values:

- `conversation_id` and `run_id`;
- the optional `before_step` boundary;
- canonical `source_ids`;
- exact `decision_ids`.

Delivery never accepts a local path and has no attachment upload/download or
BLOB table. A source-owner verifier must confirm existence, Workspace scope,
current reader access and the run boundary before `feature.discovery.replace`
can commit. Feature confirmation copies the verified lineage into the immutable
FeatureRevision; it cannot later be replaced.

## Delivery lifecycle

1. PM updates one mutable Feature discovery workspace and distinguishes user
   facts, inference, conflicts, assumptions, options and explicit decisions.
2. A human confirms the ready workspace into an immutable FeatureRevision.
3. RD starts one DeliveryRun from the exact installed ProductRevision baseline.
4. DeliveryUnits progress through interaction, model, backend, frontend,
   contract/gap and journey phases. The verification adapter submits typed
   Plane-guide evidence for one clean Git revision.
5. RD records the executable ProductRevision candidate. QA and human business
   acceptance bind to that same revision.
6. OP configures release checks; trusted deployment infrastructure records the
   original provider attempt and reconciles unknown outcomes when necessary.
7. A successful release atomically appends ProductStory + ProductDefinition as
   the next ProductRevision, advances current revisions, installs the Feature
   and completes the run.

Quality or acceptance failure routes back to a concrete DeliveryUnit phase.
Fixes require a new integrated Git revision, which invalidates the old evidence.

## Authority

- Agent actors can perform only catalog actions projected for agents.
- Feature confirmation, acceptance and release ownership require an
  authenticated human with the relevant assignment.
- Runtime verification and deployment evidence require a trusted system
  principal with `delivery_deployment.record`.
- Identity is resolved server-side; callers cannot declare actor kind,
  permissions, Workspace, state or status.
