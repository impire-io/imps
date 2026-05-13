package imps

// launchReasoning runs the user's reasoning function in a fresh goroutine.
// The InflightReasoning gauge is incremented before the goroutine starts
// and decremented in a deferred recover. A recovered panic increments
// ReasoningPanics; a returned error increments ReasoningErrors.
//
// The shutdown WaitGroup is incremented synchronously so Shutdown can
// wait on it; the goroutine signals Done in the same defer that
// decrements the gauge.
//
// The ctx passed to the reasoning function is the imp's reasoning
// context — cancelled when shutdown begins so reasoning can exit
// cooperatively.
func (i *Imp) launchReasoning(reason any, entity Entity) {
	i.runtime().metrics.InflightReasoning.Add(1)
	i.runtime().reasoningWG.Add(1)
	go func() {
		defer i.runtime().reasoningWG.Done()
		defer i.runtime().metrics.InflightReasoning.Add(-1)
		defer func() {
			if r := recover(); r != nil {
				i.runtime().metrics.ReasoningPanics.Add(1)
				i.runtime().logger.error("reasoning panic",
					"entity", string(entity),
					"recovered", r,
				)
			}
		}()
		err := i.spec.Reasoning(i.runtime().reasoningCtx, reason, entity, i.runtime().reasoning)
		if err != nil {
			i.runtime().metrics.ReasoningErrors.Add(1)
			i.runtime().logger.warn("reasoning error",
				"entity", string(entity),
				"err", err,
			)
		}
	}()
}
