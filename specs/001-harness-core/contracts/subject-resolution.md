# Contract: Subject Resolution

How declared channel and action subjects map to the resolved NATS subjects the harness actually subscribes/publishes on, in both deployment modes. Honors `docs/02-capability-service-pattern.md` and FR-030 / FR-031 / FR-032 / FR-033.

---

## Inputs

The harness construction receives:

- `prefix` — a non-empty subject prefix (string). Required in both modes.
- `platform_mode` — boolean.
- `importer_account_pk` — public key string. Required when `platform_mode = true`; ignored when `platform_mode = false`.

Per-call inputs:

- `declared` — the developer's declared subject (from `ChannelSpec.SubjectSource.Subject`, `ChannelSpec.StreamSource.FilterSubject`, or `ImpSpec.Actions[i]`).

---

## Resolution rules

### Non-platform mode (`platform_mode = false`)

```
resolved = prefix + "." + declared
```

Example:
- prefix = `tenantA.imps.demo`
- declared = `messages.in`
- resolved = `tenantA.imps.demo.messages.in`

(User Story 7 acceptance scenario 1.)

### Platform mode (`platform_mode = true`)

```
resolved = prefix + "." + importer_account_pk + "." + declared
```

Example:
- prefix = `platform`
- importer_account_pk = `ABCD1234EFGH`
- declared = `actions.out`
- resolved = `platform.ABCD1234EFGH.actions.out`

(User Story 7 acceptance scenario 2; capability-service-pattern subject convention.)

The `importer_account_pk` segment lives in a single fixed position (immediately after the prefix). The resolver does NOT support per-channel or per-action overrides of position in v1.

---

## Symmetry guarantees

1. **Channels and actions resolve identically.** Within a single run, channel subscription subjects and action publish subjects pass through the same resolver. (FR-031: "the resolved-subject convention MUST be identical for channels and actions within a single run".)

2. **Source code is mode-independent.** The imp's spec contains pre-resolution subjects only. Switching `platform_mode` and the mode-specific parameters does not require source changes (FR-032, SC-008).

3. **Whitelist is on declared subjects.** The action whitelist (`ImpSpec.Actions`) contains pre-resolution subjects. The reasoning context's `Publish(ctx, subject, payload)` is called with a declared subject — the resolver runs after the whitelist check passes (FR-027 + R-12).

---

## Failure modes

| Condition | Error | Trigger point |
|---|---|---|
| `platform_mode = true` and `importer_account_pk = ""` | `ErrConfigInvalid{Field: "importer_account_pk"}` | At `Run`, before any subscription is established (FR-033, edge case). |
| `prefix = ""` | `ErrConfigInvalid{Field: "prefix"}` | At `Run`. |
| `declared = ""` (channel or action) | `ErrSpecInvalid{Field: "channel/action subject", Reason: "empty"}` | At `NewImp` validation. |

---

## Wildcard / pattern behavior

NATS subject patterns (`*`, `>`) are passed through verbatim by the resolver. Pattern handling is the substrate's responsibility.

For channel `SubjectSource.Subject = "events.*.created"` in non-platform mode with prefix `tenantA`, resolved = `tenantA.events.*.created`. The harness subscribes on that resolved pattern.

For platform mode, the same channel resolved = `<prefix>.<importer_pk>.events.*.created`. The wildcard remains in the same relative position.

The action whitelist (FR-027) does NOT match wildcards in v1 — whitelist membership is exact set membership. A reasoning Publish to a subject that matches a whitelist pattern but is not literally in the whitelist returns `ErrWhitelistViolation`. (Spec Assumption: whitelist semantics are set membership.)

---

## Behavior changes when the resolver is replaced

If a future feature introduces additional path positions (e.g., a soulstream segment, a tenant override), the resolver type signature stays the same — `Resolve(declared string) string` — and only the implementation changes. Channels, actions, and any new emit paths continue to share the resolver, preserving FR-031's symmetry guarantee.
