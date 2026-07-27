# Can schedule channels ride the existing seam on a real server-side primitive?

**State:** active
**Started:** 2026-07-27

## Abstract

M3 — schedule channels — is the roadmap's next milestone: periodic work as a
channel kind fed by **NATS server-side scheduling**, with TTLs governing
whether stale ticks accumulate across long sleeps (vision; anatomy, Channels
`[D]`). Its gate has an external half — "the server-side scheduling
primitive available on the target substrate" — that has never been pinned:
does the substrate version imps actually depends on (`nats-server v2.14.0`)
provide server-side scheduling at all, and in what shape? This topic pins
that primitive from the pinned server's own source and behavior (not
documentation folklore), spikes schedule-fed dispatch through the existing
channel seam warm and cold, measures TTL-governed stale-tick expiry, and —
applying the episode-0007 lesson — inventories the sibling projects' scopes
so no boundary is drawn blind. A decisive answer graduates into the
`02-DESIGN/` doc that clears M3's gate; a missing primitive is a
vision-level finding, recorded as such.

## The question

Does the pinned substrate provide a server-side scheduling primitive that
can feed periodic ticks into an imp's **existing** channel kinds — dispatch
identical to any other channel, firing whether the imp is warm or cold, with
stale-tick accumulation governed by an explicit TTL — and what minimal
surface (if any) does imps need to expose it?

## Pre-registered bars

- **Bar 1 — the primitive pinned to the substrate.** What `nats-server
  v2.14.0` (the version in the core `go.mod`) actually provides for
  server-side scheduling and per-message TTL, evidenced from the pinned
  module's source and/or live behavior on the embedded server — including
  the exact configuration/headers/API and any prerequisites. *Pass:* the
  primitive's existence and shape are `[measured]` (a working reading on the
  embedded server), or its absence is established the same way. *Fail:* any
  claim sourced from docs folklore or memory alone.
- **Bar 2 — schedule ticks ride the seam unchanged.** A scratchpad spike: a
  registered server-side schedule fires ticks onto a subject an imp consumes
  through an **existing** channel kind; dispatch is identical to any other
  channel (byte-identical harness); a **cold** imp (not running while ticks
  fire) receives on next start exactly what the TTL policy says it should.
  *Pass:* warm delivery + cold catch-up measured with the harness working
  tree byte-identical. *Fail:* the spike needs a harness change or a new
  dispatch path.
- **Bar 3 — stale-tick accumulation governed by an explicit TTL.** With a
  TTL configured, ticks that expire while the imp is cold are *not*
  delivered on wake; unexpired ticks are; with no TTL, all ticks accumulate.
  *Pass:* both behaviors measured with exact counts. *Fail:* expiry is
  probabilistic, unconfigurable, or requires imp-side filtering to fake.
- **Bar 4 — sibling scopes inventoried (the episode-0007 corrective).** What
  soulstream and soulrealm declare — or are silent on — about scheduling and
  periodic work, with `file:line` evidence, before any imps-side surface is
  designed. *Pass:* both siblings pinned. *Fail:* the design proceeds on an
  unchecked boundary.

## Reversal condition

If Bar 1 establishes that the pinned substrate has **no** usable server-side
scheduling primitive (and none behind a feature flag or minor upgrade), the
vision's "Periodic work uses NATS server-side scheduling" commitment is the
thing under test, not the milestone: the finding goes to the owner as a
vision-level decision (external scheduler capability vs. vision amendment)
rather than being worked around quietly.

## Verdict

<Empty until graduation. Filled by /research-graduate: PASS/FAIL per bar with
the honest numbers, each load-bearing claim tagged [measured] /
[mechanism-argument] / [judgment].>
