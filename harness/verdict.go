package harness

// verdictKind is the unexported discriminator for Verdict. User code cannot
// construct a Verdict with an arbitrary kind because the field is unexported
// and the type is a struct (not an interface).
type verdictKind uint8

const (
	verdictIgnore verdictKind = iota + 1
	verdictNote
	verdictWake
)

// Verdict is the closed sum returned by an awareness function. It is one of
// Ignore, Note(payload), or Wake(reason, entity). Construct only via the
// exported Ignore, Note, Wake constructors.
type Verdict struct {
	kind    verdictKind
	payload any
	reason  any
	entity  Entity
}

// Ignore returns a Verdict that produces no further side effect. Dispatch
// returns immediately; no reasoning is queued and no Note is recorded.
func Ignore() Verdict {
	return Verdict{kind: verdictIgnore}
}

// Note returns a Verdict that delivers the payload to the imp's OnNote hook
// if registered, and records the verdict in NotesDelivered. No reasoning is
// queued.
func Note(payload any) Verdict {
	return Verdict{kind: verdictNote, payload: payload}
}

// Wake returns a Verdict that queues reasoning asynchronously with the
// supplied reason and entity. Channel dispatch returns before reasoning
// runs. The entity carried by Wake may differ from the message entity —
// awareness is allowed to redirect reasoning to a different entity.
func Wake(reason any, entity Entity) Verdict {
	return Verdict{kind: verdictWake, reason: reason, entity: entity}
}
