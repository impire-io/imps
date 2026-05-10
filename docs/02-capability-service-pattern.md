# Capability Service Pattern

*The shared deployment shape every capability service follows. Wire protocols are each capability's own design; this document specifies what they have in common.*

---

A capability service is a NATS micro service that handles a specific kind of work for imps — inference, knowledge, tool execution, or anything else the harness needs to reach for during reasoning. The framework defines no central registry of capabilities; the live capability surface of a deployment is whatever NATS micro services are reachable at a given moment.

What's standardized is the *shape*: how a capability service registers, how imps discover it, where it sits in the subject hierarchy, what metadata it carries, and what operational invariants it satisfies. What's *not* standardized is the wire protocol — endpoints, request/response shapes, error semantics. Those are each capability's own design, recorded in its own spec.

This document is the deployment shape. The inference service spec is the canonical worked example.

## NATS micro registration

Every capability service registers as a NATS micro service. The standard `$SRV.PING`, `$SRV.INFO`, and `$SRV.STATS` interfaces are how clients discover, health-check, and observe the service. Custom discovery mechanisms are not allowed; capability services that want to be reachable use NATS micro and only NATS micro.

Each running instance of a capability service is one NATS micro service registration. Multiple instances with the same service name form a queue group — requests on the service's subjects load-balance across them. Multiple instances with different service names coexist as heterogeneous siblings, each handling its own subset of work.

The service `name` and `version` are configuration. The instance ID is assigned by NATS micro at registration time.

## Endpoints carry the subject contract

A capability service registers one or more endpoints. Each endpoint is bound to a specific NATS subject and handles requests on that subject. The subject is the contract — imps address work by publishing on subjects, not by looking up service names.

Per-endpoint metadata in `$SRV.INFO` carries everything an imp needs to know to use the endpoint:

- The subject the endpoint handles.
- A request schema and a response schema.
- A **bounded** flag, indicating whether the endpoint guarantees a bounded response envelope (single round-trip, deterministic latency budget, no fan-out, no side effects beyond the call). Bounded endpoints are awareness-callable; unbounded endpoints are reasoning-only.
- Any capability-specific metadata the endpoint wants to expose.

Service-level metadata in `$SRV.INFO` carries:

- Descriptive capability labels (e.g. `["vision", "reasoning"]` for inference) — these are for human and operator-facing filtering, not routing.
- Service-wide configuration hints relevant to consumers (deployment mode, supported variants, etc.).
- Anything else that applies to the service as a whole rather than to individual endpoints.

Imps discover by querying `$SRV.INFO`, walking endpoint lists across replies, and checking declared subjects against their spec dependencies. The harness does this once at startup and exposes the resolved capability surface to the imp; imps do not query at request time.

## Subject prefix convention

Every subject a capability service registers an endpoint on follows a prefix convention determined by deployment mode.

**Non-platform mode** — the service runs in an account where its consumers also live. Subjects are prefixed with a configured `<prefix>`:

```
<prefix>.<capability-specific-tokens>
```

Example: `app.knowledge.episode.recall`, where `app` is the configured prefix.

**Platform mode** — the service runs in a platform account and is exported to other accounts that import it. The calling account's public key is part of the subject path so the platform service can attribute every request:

```
<prefix>.<account-pub-key>.<capability-specific-tokens>
```

Example: `platform.ABCD1234EFGH.knowledge.episode.recall`, where `ABCD1234EFGH` is the importing account's public key.

Both modes are served by the same binary, configured by a `platform_mode` boolean. Endpoints, wire protocols, and discovery surfaces are identical across modes; only the subject path and the per-request account attribution differ.

After the prefix, the subject taxonomy is each capability's own design. Inference uses `inference.<service-name>.{prompt,embed}`. Knowledge will use `knowledge.<variant>.{recall,remember}`. The framework does not dictate the post-prefix structure beyond the recommendation that the first post-prefix token name the capability family.

## Statelessness per request

Capability service instances are stateless with respect to per-request data. No request payload, no response payload, no client-supplied content persists on the instance beyond the request's lifetime.

Audit/log records emitted per request are explicitly excluded from this constraint, but they must not contain content beyond what's needed for cost attribution and lifecycle reconstruction.

This invariant is what lets capability services scale horizontally without coordination, restart freely without state migration, and be terminated and replaced at any time. The next `$SRV.INFO` request reflects the live cluster without heartbeats, registry writes, or client cache invalidation.

## Configuration determines what an instance offers

A capability service instance is configured at startup with what it serves. Configuration determines:

- Which endpoints are registered (and therefore which subjects the instance handles).
- What metadata each endpoint declares — schemas, boundedness, capability-specific hints.
- What scope or backing the instance uses (a knowledge instance configured for tenant-wide episodic memory backed by Elasticsearch; a different instance configured for imp-class-scoped procedural memory backed by a vector store).

Heterogeneity at the cluster level is achieved by running different instances with different configurations, not by multiplexing inside an instance. Two instances of the same capability service can offer non-overlapping endpoint sets, reflect different scopes, or use different backing stores. The discovery surface tells imps what each one does.

A consequence: scope is a deployment concern, not a wire protocol concern. The protocol does not need a "scope" parameter on every request; the subject prefix and the configured instance behind it carry the scoping naturally.

## HA and load balancing

Multiple instances configured identically register on the same service name and the same endpoint subjects. NATS queue-group semantics distribute requests across them. Failure of one instance does not interrupt service; new instances added to the group immediately receive their share of traffic.

For capabilities where state must be partitioned (a knowledge service backed by a sharded store, for example), the partitioning lives inside the capability implementation and is invisible to the wire protocol. The capability service spec defines whatever consistency guarantees apply; the framework does not impose its own.

## Discovery is at the door, not on every call

The harness queries `$SRV.INFO` at imp startup and aggregates declared endpoints into the imp's resolved capability surface. Required dependencies that aren't satisfied cause the imp to fail to start with a clear error. Optional dependencies that aren't satisfied are recorded as flags the imp can check at runtime if it adapts behavior.

After startup, the imp addresses capabilities by subject directly. There is no per-request discovery, no name lookup, no client-side service selection. If a service goes away while an imp is running, calls to its subjects time out — the imp handles that as a degradation, not a discovery refresh.

This trades adaptability for simplicity, deliberately. An imp that wants to handle "is this capability available right now" cases asks the harness; the harness can re-query `$SRV.INFO` if the imp explicitly requests it. The default is "discovery is verification at startup, addressing is by subject thereafter."

## Audit and statistics

Every capability service instance:

- Reports health via `$SRV.PING`. Healthy means the service is running, has loaded any external dependencies it needs (provider clients, backing stores), has registered with NATS micro, and has subscribed to its supported endpoint subjects.
- Reports per-endpoint statistics via `$SRV.STATS`, augmented with capability-specific counters in the standard `data` field.
- Emits per-request audit records sufficient to attribute work to a caller and reconstruct the request lifecycle. Audit records do not contain request or response content beyond what's needed for that purpose.

The framework does not centralize audit storage or statistics aggregation. Each capability emits records; downstream consumption (dashboards, compliance archives, cost attribution) is operator infrastructure, not framework concern.

## Versioning the contract

Once an imp depends on a subject, the subject's wire protocol is a public API. Capability service authors version their endpoints and treat changes as breaking changes accordingly.

Two patterns the framework recommends but does not mandate:

- **Token-position versioning** — include a version segment in the subject (`knowledge.v1.episode.recall`). New major versions register on parallel subjects; old versions stay supported until consumers migrate.
- **Schema versioning in metadata** — the endpoint's request and response schemas in `$SRV.INFO` carry a version. Imps check at startup that the schema version matches what their spec was written against.

Either is fine. The principle is that subjects are the contract and the contract is versioned; how the versioning is expressed is per-capability.

## What this pattern does not specify

Deliberately left to each capability:

- Endpoint count, names, and shapes.
- Request and response schemas.
- Error categories and their meaning.
- Streaming versus single-shot wire patterns. (Inference uses streaming-with-empty-terminator; knowledge will likely use single-shot request/reply; tools may use either.)
- Boundedness criteria — the capability decides which of its endpoints are bounded and what the bound actually means quantitatively.
- Configuration schema beyond what's needed for the deployment shape.
- Backing store choices, internal architecture, scaling strategies.

Each capability spec specifies these for itself. The pattern is the operational shape; the wire protocol is the capability's own design.

## A worked example: inference

The inference service spec ([reference]) implements this pattern:

- Registers as a NATS micro service with a configurable `name`.
- Two endpoints (`prompt`, `embed`), each on its own subject.
- Service-level metadata declares capability labels (`vision`, `reasoning`, etc.) for descriptive filtering.
- Endpoint-level metadata declares request/response schemas. Whether `embed` is flagged bounded for awareness use is the inference spec's call; given its single-shot, deterministic nature, it makes sense.
- Stateless per request; no prompt or response content persists on the instance.
- Heterogeneity is one model per instance; running multiple instances with different models gives the cluster heterogeneous capabilities.
- Subject prefix convention follows the platform/non-platform split.
- HA via queue groups with the same service name.

Knowledge will follow the same shape with different endpoints, different schemas, different scoping configuration. So will future capabilities.

## Decisions and tradeoffs

- **No generic capability protocol.** Each capability defines its own wire protocol. Considered and rejected: a uniform request/response schema. The result would have been a lowest-common-denominator surface that fits no capability well.
- **Endpoint metadata, not service-level metadata, carries the per-subject contract.** The subject lives on the endpoint already; duplicating it elsewhere creates drift. Boundedness, schemas, and per-subject hints all live with the endpoint.
- **Discovery at startup, addressing by subject afterward.** Trades adaptability for simplicity. Imps that need adaptation can ask the harness to re-query, but it's an exception path.
- **Scope is configuration, not a wire-protocol parameter.** Pushes scoping into deployment topology. Cleaner protocols, no scope-validation complexity, natural multi-tenancy via NATS account scoping.
- **Statelessness per request is invariant.** Coordination state (in-flight tracking, replay buffers) lives on callers, not on capability instances. This is what makes free restart and horizontal scaling possible.
- **Versioning is the capability's responsibility, not the framework's.** The framework doesn't impose a version scheme because different capabilities will want different ones; documenting both common patterns suffices.

The pattern is deliberately small. A document longer than this would mean the framework is doing too much.