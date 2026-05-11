package harness

// awarenessCtx is the concrete AwarenessContext. It exposes only State —
// no Publish — which is the structural enforcement of the energy
// gradient. Reasoning's Publish lives on its own type (reasoningCtx).
type awarenessCtx struct {
	registry *registry
}

func (a *awarenessCtx) State(name string, entity Entity) (StateRef, error) {
	return a.registry.ref(name, entity)
}
