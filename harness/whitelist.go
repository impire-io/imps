package harness

// whitelist is the set view of ImpSpec.Actions. Membership is exact:
// wildcard subjects are not expanded (FR-027 + spec assumption that
// whitelist semantics are set membership). Built once at NewImp time and
// read-only thereafter, so no synchronization is required.
type whitelist struct {
	subjects map[string]struct{}
}

func newWhitelist(actions []string) *whitelist {
	w := &whitelist{subjects: make(map[string]struct{}, len(actions))}
	for _, s := range actions {
		w.subjects[s] = struct{}{}
	}
	return w
}

// check returns nil if the declared subject is whitelisted, or a typed
// *ErrWhitelistViolation otherwise.
func (w *whitelist) check(subject string) error {
	if _, ok := w.subjects[subject]; ok {
		return nil
	}
	return &ErrWhitelistViolation{Subject: subject}
}
