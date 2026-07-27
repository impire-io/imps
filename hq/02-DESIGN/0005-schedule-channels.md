# Schedule Channels

Periodic work as ticks an imp consumes through its existing channels: the
substrate produces them (JetStream message scheduling, present in the pinned
`nats-server v2.14.0`), an explicit TTL governs stale-tick accumulation
across long sleeps, and imps adds only a thin convenience surface. This
document is the M3 design (roadmap, "Schedule channels"), graduated from the
`schedule-channels` research topic
([episode 0008](../04-JOURNEY/0008-schedule-channels.md)). Everything here
is **[V]** — shipped as feature `006-schedule-channels`
([episode 0009](../04-JOURNEY/0009-schedule-channels-shipped.md)); this
document describes the package as built and tested. Implementation drift,
propagated: the server's cron grammar is **six-field with a seconds field**
("0 0 12 * * *"), alongside `@every <dur>` (minimum 1 s), `@at <RFC3339>`
one-shots, and the predefined `@hourly`…`@yearly` forms; and `Tick.Next` is
the header verbatim (RFC3339 on repeating schedules, the server's purge
marker on final firings).

The load-bearing finding this design rests on `[measured]`: the spike ran
warm delivery, cold durable catch-up, and TTL-governed expiry through the
**existing `StreamSource`** with the harness byte-identical. The framework
does not produce ticks, does not run timers, and gains no channel kind — the
server owns the clock.

## The substrate contract (owned by nats-server; consumed, not defined)

- A **schedule** is a message in a stream configured with
  `AllowMsgSchedules: true`, carrying `Nats-Schedule` (`@every <dur>` — min
  1 s —, `@at <RFC3339>`, six-field cron with seconds, or a predefined
  `@hourly`-style form; optional `Nats-Schedule-Time-Zone`, cron only), `Nats-Schedule-Target`
  (the subject ticks are emitted to), and optionally `Nats-Schedule-TTL`
  (stamps every tick `Nats-TTL`, so the server expires stale ticks —
  requires `AllowMsgTTL: true` on the stream), `Nats-Schedule-Rollup`, and
  `Nats-Schedule-Source` (emit the last message on a source subject instead
  of the schedule's own body).
- A **tick** is an ordinary message appended to the stream on the target
  subject, scheduling headers stripped, provenance added: `Nats-Scheduler`
  (the schedule's subject) and `Nats-Schedule-Next`.
- One schedule per schedule-subject; re-publishing replaces it. Removal is
  removing the schedule message.

## Where it lives: `imps/schedule`, a package in the core module

Like `persist`: a plain package, **zero new dependencies** (`jetstream` is
already required), zero root-package changes, the existing gate covers it.
An imp that doesn't schedule links nothing new.

## Surfaces

```go
package schedule // import "github.com/impire-io/imps/schedule"

// Tick is the imp's header-level view of one schedule firing.
type Tick struct {
    Subject   string // the target subject the tick arrived on
    Scheduler string // Nats-Scheduler: the schedule subject that produced it
    Next      string // Nats-Schedule-Next, verbatim: RFC3339 of the next firing, or the server's purge marker on a final firing
}

// Channel returns a standard harness ChannelSpec consuming the ticks on
// target from stream: the EXISTING StreamSource, deliver-all by default,
// with a header-only Tick decoder and the target subject as the entity.
// Durable naming (resume across restarts, with the server having already
// expired what the schedule's TTL says is stale) and start-position,
// decoder, extractor, and name overrides pass through exactly as in
// soulstream.TopicChannel and the harness's own consumer config.
func Channel(stream, target string, opts ...ChannelOption) imps.ChannelSpec

func WithDurable(name string) ChannelOption
func WithStartSeq(seq uint64) ChannelOption
func WithStartTime(t time.Time) ChannelOption
func WithDecoder(d imps.Decoder) ChannelOption
func WithEntityExtractor(e imps.EntityExtractor) ChannelOption
func WithName(name string) ChannelOption

// Register publishes (or replaces — one schedule per subject) a schedule:
// a headered message built for you, so the six magic headers are typed.
// Thinking-tier or operator-tier only; never awareness.
func Register(ctx context.Context, js jetstream.JetStream, scheduleSubject, pattern, target string, opts ...RegisterOption) error

func WithTickTTL(ttl time.Duration) RegisterOption // stale-tick governor (stream needs AllowMsgTTL)
func WithTimeZone(tz string) RegisterOption        // cron patterns only
func WithBody(body []byte) RegisterOption          // the tick payload (default empty)
func WithSource(subject string) RegisterOption     // emit the last message on subject instead of Body
func WithRollup() RegisterOption                   // tick carries a per-subject rollup

// Deregister removes the schedule (purges the schedule subject). Ticks
// already emitted are unaffected.
func Deregister(ctx context.Context, js jetstream.JetStream, stream, scheduleSubject string) error
```

Contractual behavior:

- `Channel` introduces no channel kind and performs no subject rewriting;
  the returned spec's source is the existing `imps.StreamSource`, and
  unknown headers or payloads flow to awareness undamaged.
- The default decode reads headers only (`Nats-Scheduler`,
  `Nats-Schedule-Next`); payload decoding is an override, exactly as in the
  soulstream module.
- `Register` validates the pattern shape client-side only minimally (the
  server is the authority); it never creates or reconfigures the stream.
- The package MUST NOT run timers, produce ticks, or poll — the server owns
  the clock. Warm imps receive ticks live; cold imps catch up via their
  durable consumer, the backlog already TTL-pruned server-side.

## Boundaries (MUST / MUST NOT)

- The harness core MUST NOT change; `compile-deny` invariants untouched.
- Registration and deregistration are thinking-tier or operator acts;
  awareness MUST NOT register schedules (it has no publish surface, and the
  package documentation forbids handing it a `jetstream` handle).
- Stream provisioning (`AllowMsgSchedules`, `AllowMsgTTL`, subject capture
  for schedules and ticks) is the operator's act; the package never
  provisions (consistent with `persist` and the soulstream module).
- Stale-tick policy MUST be explicit: the design default is
  **TTL-required-by-convention** — `Register` without `WithTickTTL` is
  legal (some schedules genuinely want full accumulation, e.g. audit
  ticks), but the package documentation makes the accumulation consequence
  explicit at the call site.

## Acceptance criteria

1. A registered schedule fires into an imp declaring `schedule.Channel`:
   live ticks while warm, durable catch-up after a cold gap, dispatch
   identical to any channel — the research spike productized, harness
   byte-identical (`git diff` empty on root, `go.mod` unchanged).
2. With `WithTickTTL`, ticks that outlive the TTL while the imp is cold are
   never delivered on wake; without it, the full backlog arrives — both
   directions measured with counts.
3. Ticks carry provenance: the default decoder yields the producing
   schedule's subject on every tick.
4. `Register` twice on one subject replaces (next firing follows the new
   pattern); `Deregister` stops future firings without disturbing emitted
   ticks.
5. The full gate passes with the new package covered; zero skipped tests.

## Decisions and tradeoffs

- **A thin package over documentation-only.** The spike needed zero imps
  code, but documentation-only leaves six stringly-typed magic headers and
  a provenance decode to every imp author — the error class
  `soulstream.TopicChannel` exists to remove. The package is sugar plus
  typed headers, nothing more; rejecting it was argued in the topic journey.
- **No schedule ownership in the framework.** The server keys schedules by
  subject and replaces on re-publish; imps adds no registry, no reconciler,
  no janitor. If an imp author wants declarative schedule state, that is an
  operator tool's job.
- **Complementary, not competing, with the siblings** (episode 0008's
  inventory): soulrealm's "scheduling" is workload placement; a schedule
  tick could someday be the trigger that launches a soulrealm `job` — same
  primitive, both projects consuming it, neither owning the other's half.
- **M2b interaction:** schedule ticks fire whether the imp is warm or cold
  by construction (the server holds them), so nothing here waits on
  whole-imp snapshot sleep; when M2b lands, TTL-pruned catch-up is exactly
  what a woken imp replays.
