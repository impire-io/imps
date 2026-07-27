# Contract: Exported Go API of `github.com/impire-io/imps/schedule`

The complete exported surface. Anything not listed is unexported or does not
ship in this feature.

```go
package schedule // import "github.com/impire-io/imps/schedule"

// Tick is the imp's header-level view of one schedule firing.
type Tick struct {
    Subject   string // target subject the tick arrived on
    Scheduler string // Nats-Scheduler: the schedule subject that produced it
    Next      string // Nats-Schedule-Next, verbatim: RFC3339 of the next firing, or the server's purge marker on a final firing
}

// Channel returns a standard harness ChannelSpec consuming ticks on target
// from stream: the EXISTING StreamSource, deliver-all by default, header-only
// Tick decoder, target subject as entity, name "schedule:"+target.
func Channel(stream, target string, opts ...ChannelOption) imps.ChannelSpec

type ChannelOption func(*channelConfig)
func WithDurable(name string) ChannelOption
func WithStartSeq(seq uint64) ChannelOption
func WithStartTime(t time.Time) ChannelOption
func WithDecoder(d imps.Decoder) ChannelOption
func WithEntityExtractor(e imps.EntityExtractor) ChannelOption
func WithName(name string) ChannelOption

// Register publishes (or replaces — one schedule per subject, the server's
// semantics) a schedule. pattern and target must be non-empty; pattern
// semantics ("@every <dur>", cron) are the server's authority. Thinking-tier
// or operator-tier only; never reachable from awareness.
func Register(ctx context.Context, js jetstream.JetStream, scheduleSubject, pattern, target string, opts ...RegisterOption) error

type RegisterOption func(*registerConfig)
func WithTickTTL(ttl time.Duration) RegisterOption // ticks expire server-side after ttl (stream needs AllowMsgTTL); ttl must be > 0; ABSENCE means full accumulation
func WithTimeZone(tz string) RegisterOption        // cron patterns only
func WithBody(body []byte) RegisterOption          // tick payload (default empty)
func WithSource(subject string) RegisterOption     // tick carries the last message on subject instead of Body
func WithRollup() RegisterOption                   // tick carries a per-subject rollup

// Deregister purges the schedule subject in stream, stopping future
// firings; ticks already emitted are unaffected.
func Deregister(ctx context.Context, js jetstream.JetStream, stream, scheduleSubject string) error
```

Contractual behavior:

- `Channel` introduces no channel kind, rewrites no subject, filters
  nothing; the default decoder reads headers only.
- `Register` writes exactly the headers its options imply and no others;
  fails fast (no substrate contact) on empty pattern/target or non-positive
  TTL. The header names are defined once, in this package.
- The package runs no timers, produces no ticks, polls nothing, provisions
  nothing, and holds no schedule state.
- `doc.go` MUST state: the server owns the clock; registration is
  thinking/operator-tier (never hand awareness a jetstream handle); the
  TTL-absence consequence (full accumulation); provisioning is the
  operator's act.
