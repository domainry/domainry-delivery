# Repository instructions

- Base every design and answer on the current `domainry-agent`, `domainry-deck`, and repository code. Do not invent interfaces from memory.
- Do not use worktrees or create development branches. Do not use a Domainry builder or static-product-ui skill.
- Delivery owns the archived PM conversation messages, original Feature attachment bytes and metadata, their verifiable source references, Product, ProductRevision, Feature, and the complete delivery lifecycle. PM/RD/QA/OP execute inside Deck; no remote Agent service is required.
- ProductStory and ProductDefinition must be committed in one immutable ProductRevision. Narrative text is not an executable definition.
- FeatureRevision must preserve the exact Conversation, Run, source, and decision references that formed the requirement.
- PM, RD, QA, and OP are local Agent Runtime roles, not remote Agent services required by this repository.
- The domain layer validates every state transition. HTTP clients never write a status directly.
- Every write requires `client_id` and `expected_revision`; the service handles idempotency and optimistic concurrency in one transaction.
- The project has not shipped. Do not add compatibility layers for old PM references or old HTTP contracts.
