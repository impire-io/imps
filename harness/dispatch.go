package harness

import (
	"context"
)

// dispatch runs the per-message pipeline: Decode → ExtractEntity →
// safeAwareness → branch on verdict. It does NOT ack/NAK; that is the
// caller's job (subject channels have no ack; stream channels ack/NAK
// outside this function based on the dispatchOutcome we return).
//
// The dispatch path returns BEFORE reasoning runs — Wake schedules
// reasoning on a fresh goroutine via launchReasoning and returns
// immediately. This is the energy gradient enforced at the call site.
type dispatchOutcome int

const (
	dispatchOK dispatchOutcome = iota
	dispatchDecodeFail
	dispatchExtractFail
	dispatchAwarenessPanic
)

// dispatch runs a single message through the pipeline. ctx is the
// per-message dispatch context — short-lived; the awareness function MUST
// NOT spawn goroutines that outlive the call.
func (i *Imp) dispatch(ctx context.Context, ch *channelState, msg Message) dispatchOutcome {
	decoded, err := ch.spec.Decode(msg)
	if err != nil {
		i.runtime().metrics.DecodeFailures.Add(1)
		i.runtime().logger.warn("decode failure",
			"channel", ch.spec.Name,
			"subject", msg.Subject,
			"err", err,
		)
		return dispatchDecodeFail
	}

	entity, err := ch.spec.ExtractEntity(decoded)
	if err != nil || entity == "" {
		i.runtime().metrics.ExtractionFailures.Add(1)
		i.runtime().logger.warn("extraction failure",
			"channel", ch.spec.Name,
			"err", err,
		)
		return dispatchExtractFail
	}

	verdict, panicked := i.safeAwareness(ctx, decoded, entity)
	if panicked {
		i.runtime().metrics.AwarenessPanics.Add(1)
		return dispatchAwarenessPanic
	}

	switch verdict.kind {
	case verdictIgnore:
		i.runtime().metrics.IgnoredVerdicts.Add(1)
	case verdictNote:
		i.runtime().metrics.NotesDelivered.Add(1)
		i.invokeNote(entity, verdict.payload)
	case verdictWake:
		i.runtime().metrics.WakesDispatched.Add(1)
		i.launchReasoning(verdict.reason, verdict.entity)
	}
	return dispatchOK
}

// safeAwareness runs the user's awareness function under a panic guard.
// The recovered panic is logged at ERROR; the caller increments the
// AwarenessPanics counter when panicked is true.
func (i *Imp) safeAwareness(ctx context.Context, decoded any, entity Entity) (v Verdict, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			i.runtime().logger.error("awareness panic",
				"entity", string(entity),
				"recovered", r,
			)
		}
	}()
	v = i.spec.Awareness(ctx, decoded, entity, i.runtime().awareness)
	return v, false
}
