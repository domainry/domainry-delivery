# Domainry Delivery TODO

## Confirmed product boundary

- [x] `Product` is the long-lived aggregate root; Story does not replace Product.
- [x] `ProductStory` is the human narrative; a released `ProductDefinition` describes an executable product baseline.
- [x] PM owns Feature requirements, not ProductModel or ProductRevision generation.
- [x] One `Feature` represents one complete PM conversation requirement; PM turns update one mutable `FeatureDraftState`, and human confirmation creates an immutable `FeatureRevision`.
- [x] PM, RD, QA, and OP are Agent Runtime roles, not remote Agent services owned by Delivery.
- [x] Delivery supports both in-process Module and SaaS bindings; Deck depends on the contract, not a cloud Agent.
- [x] `domainry-pm` is no longer a Product authority dependency.

## M1: Authoritative Product and Feature model

- [x] Implement `Product`, immutable `ProductRevision`, `ProductStory`, and `ProductDefinition`.
- [x] Implement evidence-led `FeatureDraftState` with scenarios, assumptions, conflicts, options, ranked questions, typed Product impacts, acceptance, readiness, and exact Conversation/Run lineage; freeze it into an immutable `FeatureRevision` only after human confirmation.
- [x] Implement Product creation plus Feature draft, human confirmation, delivery start, and installation lifecycles.
- [x] Validate ProductDefinition actor, scenario, object, field, state, rule, action, page, and exception references when an executable ProductRevision is submitted.
- [x] Persist Product aggregates, idempotent receipts, and optimistic revisions in SQLite.
- [x] Provide Product list, detail, command, and Agent Context HTTP APIs.
- [x] Remove hard dependencies on PM Service, PM Product ID, and PRD Revision.

## M2: Feature DeliveryRun

- [x] Replace `Iteration` with `DeliveryRun`; every run binds exactly one confirmed `FeatureRevision`.
- [x] Advance `Feature.confirmed -> delivering -> installed` only from human confirmation and real DeliveryRun outcomes.
- [x] Allow zero WorkItems for simple Features; let the RD Agent decompose complex Features as needed.
- [x] Generate TestCases from acceptance criteria; create Issues from failures and enforce fix/retest closure.
- [x] Generate environment-bound ReleaseChecks; require real Artifact, Revision, Environment, and Receipt evidence for deployment.
- [x] Atomically install a successful Feature and its live release receipt into Product.
- [ ] Add an RD-owned command that records the executable ProductRevision produced by a DeliveryRun before release.
- [ ] Advance `current_release_revision` only when that exact executable revision is installed.

## M3: Module / SaaS parity

- [x] Define one Delivery Binding contract shared by Module and SaaS.
- [x] Let Module borrow host persistence and SaaS own its database while preserving the same domain state machine.
- [x] Contract-test capability descriptors, command results, identity behavior, and error semantics across both bindings.
- [x] Resolve Identity server-side, overwrite any command actor, and reject client-declared authority.

## M4: Deck integration

- [x] Render `ProductProjection` in Product Studio without duplicating lifecycle conditions in the client.
- [x] Connect local PM/RD/QA/OP Agent roles to Product, Feature, and DeliveryRun through the Delivery Binding.
- [x] Compile pages, session data, operations, rules, and state transitions from the currently released ProductRevision.
- [x] Persist every PM turn into the same versioned Feature discovery workspace, restore it with historical Feature context, and expose human confirmation only when evidence, decisions, impacts, and acceptance are complete.
- [x] Require explicit human confirmation before exposing `feature.delivery.start`.
- [x] Create a DeliveryRun through a real in-app setup form and hand the confirmed Feature to RD.
- [x] Remove Mock Product Service, timer completion, generic fake preview, and fabricated publish URLs.
- [ ] Complete RD, QA, and OP end-to-end acceptance with a real repository revision, hashed build artifact, test evidence, executable ProductRevision, and trusted deployment receipt.

## M5: Language support

- [x] Keep stable protocol keys, command names, IDs, and enum values language-neutral.
- [x] Publish supported locale metadata and accept a locale on both Module and SaaS bindings.
- [x] Localize Delivery-owned release gate and error presentation for every supported non-Chinese Deck locale.
- [x] Keep source, seed, audit, and documentation copy free of Chinese defaults.
