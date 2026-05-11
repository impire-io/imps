package harness

// invokeNote calls the user's OnNote hook (if registered) under a panic
// guard. A nil OnNote drops the payload; a panicking OnNote is treated as
// having occurred during the dispatch path and increments AwarenessPanics
// per contracts/observability.md.
func (i *Imp) invokeNote(entity Entity, payload any) {
	if i.spec.OnNote == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			i.runtime().metrics.AwarenessPanics.Add(1)
			i.runtime().logger.error("note hook panic",
				"entity", string(entity),
				"recovered", r,
			)
		}
	}()
	i.spec.OnNote(entity, payload)
}
