# 02-DESIGN — the framework, specified

This set specifies, from a functional point of view, what the imp framework
**is** and **how each part behaves** — not the reasoning behind every choice
(that lives in each doc's "decisions and tradeoffs" section and in the
journey). An implementer should be able to build the described part from these
documents plus the constitution, without needing undocumented decisions.

**The spec-kit rule:** every document here is written explicit enough to be the
argument to `/speckit-specify` — the capability, its seams, its configuration
surface, and its acceptance criteria, with no guessing left to the spec writer.
New documents take the next free `NNNN-` number. Graduating research enters
through `/research-graduate`; behavioral changes made during implementation
propagate back here (see
[`../00-GENESIS/how-we-work.md`](../00-GENESIS/how-we-work.md)).

## Documents

| # | Document | Covers |
|---|---|---|
| 0001 | [`0001-anatomy.md`](0001-anatomy.md) | The five parts of an imp — channels, awareness, thinking, memory, action — their surfaces, invariants, and the compile-enforced boundary between awareness and thinking |
| 0002 | [`0002-capability-service-pattern.md`](0002-capability-service-pattern.md) | The shared deployment shape every capability service follows (NATS micro registration, subject convention, statelessness); wire protocols stay per-capability |

The vision ([`../00-GENESIS/vision.md`](../00-GENESIS/vision.md)) is the map;
read it first. `0001-anatomy.md` is the natural place for an implementer to
begin — it describes what is built and shipped today.

## Status legend (used throughout)

Every component and requirement carries one of these tags. They describe
**maturity**, not importance. All tagged items are mandatory unless marked
otherwise.

- **[V] Validated** — built and shipped; behavior confirmed by the test suite
  and the compile-deny invariants. Build against it; deviations must be
  justified against the shipped behavior.
- **[D] Design** — fully specified functionally, but not yet built. Build it as
  specified; expect refinement once it runs.
- **[O] Open** — the interface and a default are specified, but the best
  internal shape is a known unsolved problem. Build the interface and the
  default; expect the internal to be replaced. **[O]** items are where risk
  concentrates.

## Requirement language

- **MUST** / **MUST NOT** — mandatory / prohibited.
- **MAY** — permitted, not required.
- A value given as a *default* is the value shipped unless configuration
  overrides it.
