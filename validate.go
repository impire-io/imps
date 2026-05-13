package imps

// validateSpec runs the construction-time invariants documented on
// ImpSpec and returns the first violation as a typed error. Callers report
// the error verbatim so the offending field name surfaces.
func validateSpec(spec ImpSpec) error {
	if spec.Name == "" {
		return &ErrSpecInvalid{Field: "Name", Reason: "empty"}
	}
	if spec.Version == "" {
		return &ErrSpecInvalid{Field: "Version", Reason: "empty"}
	}
	if spec.Awareness == nil {
		return &ErrSpecInvalid{Field: "Awareness", Reason: "nil"}
	}
	if spec.Reasoning == nil {
		return &ErrSpecInvalid{Field: "Reasoning", Reason: "nil"}
	}
	if err := validateStates(spec.States); err != nil {
		return err
	}
	if err := validateChannels(spec.Channels); err != nil {
		return err
	}
	return nil
}

func validateStates(states []StateShape) error {
	seen := make(map[string]struct{}, len(states))
	for _, s := range states {
		if s.Name == "" {
			return &ErrSpecInvalid{Field: "StateShape.Name", Reason: "empty"}
		}
		if _, dup := seen[s.Name]; dup {
			return &ErrDuplicateStateShape{Shape: s.Name}
		}
		seen[s.Name] = struct{}{}
		if s.Factory == nil {
			return &ErrSpecInvalid{Field: "StateShape.Factory", Reason: "nil for shape " + s.Name}
		}
		if s.Cap <= 0 {
			return &ErrSpecInvalid{Field: "StateShape.Cap", Reason: "must be > 0 for shape " + s.Name}
		}
	}
	return nil
}

func validateChannels(channels []ChannelSpec) error {
	seen := make(map[string]struct{}, len(channels))
	for _, c := range channels {
		if c.Name == "" {
			return &ErrSpecInvalid{Field: "ChannelSpec.Name", Reason: "empty"}
		}
		if _, dup := seen[c.Name]; dup {
			return &ErrSpecInvalid{Field: "ChannelSpec.Name", Reason: "duplicate channel name " + c.Name}
		}
		seen[c.Name] = struct{}{}
		if c.Decode == nil {
			return &ErrSpecInvalid{Field: "ChannelSpec.Decode", Reason: "nil for channel " + c.Name}
		}
		if c.ExtractEntity == nil {
			return &ErrSpecInvalid{Field: "ChannelSpec.ExtractEntity", Reason: "nil for channel " + c.Name}
		}
		if c.Source == nil {
			return &ErrSpecInvalid{Field: "ChannelSpec.Source", Reason: "nil for channel " + c.Name}
		}
		if err := validateSource(c.Name, c.Source); err != nil {
			return err
		}
	}
	return nil
}

func validateSource(channelName string, src Source) error {
	switch s := src.(type) {
	case SubjectSource:
		if s.Subject == "" {
			return &ErrSpecInvalid{Field: "SubjectSource.Subject", Reason: "empty for channel " + channelName}
		}
	case StreamSource:
		if s.Stream == "" {
			return &ErrSpecInvalid{Field: "StreamSource.Stream", Reason: "empty for channel " + channelName}
		}
		if s.FilterSubject == "" {
			return &ErrSpecInvalid{Field: "StreamSource.FilterSubject", Reason: "empty for channel " + channelName}
		}
	default:
		return &ErrSpecInvalid{Field: "ChannelSpec.Source", Reason: "unknown source kind for channel " + channelName}
	}
	return nil
}
