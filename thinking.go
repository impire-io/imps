package imps

// launchThinking runs the user's thinking function in a fresh goroutine.
// The InflightThinking gauge is incremented before the goroutine starts
// and decremented in a deferred recover. A recovered panic increments
// ThinkingPanics; a returned error increments ThinkingErrors.
//
// The shutdown WaitGroup is incremented synchronously so Shutdown can
// wait on it; the goroutine signals Done in the same defer that
// decrements the gauge.
//
// The ctx passed to the thinking function is the imp's thinking
// context — cancelled when shutdown begins so thinking can exit
// cooperatively.
func (i *Imp) launchThinking(reason any, entity Entity) {
	i.runtime().metrics.InflightThinking.Add(1)
	i.runtime().thinkingWG.Add(1)
	go func() {
		defer i.runtime().thinkingWG.Done()
		defer i.runtime().metrics.InflightThinking.Add(-1)
		defer func() {
			if r := recover(); r != nil {
				i.runtime().metrics.ThinkingPanics.Add(1)
				i.runtime().logger.error("thinking panic",
					"entity", string(entity),
					"recovered", r,
				)
			}
		}()
		err := i.spec.Thinking(i.runtime().thinkingCtx, reason, entity, i.runtime().thinking)
		if err != nil {
			i.runtime().metrics.ThinkingErrors.Add(1)
			i.runtime().logger.warn("thinking error",
				"entity", string(entity),
				"err", err,
			)
		}
	}()
}
