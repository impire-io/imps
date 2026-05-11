package harness

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func minimalSpec() ImpSpec {
	return ImpSpec{
		Name:    "test",
		Version: "0.0.1",
		Awareness: func(context.Context, any, Entity, AwarenessContext) Verdict {
			return Ignore()
		},
		Reasoning: func(context.Context, any, Entity, ReasoningContext) error { return nil },
	}
}

func TestValidateSpec_HappyPath(t *testing.T) {
	if err := validateSpec(minimalSpec()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateSpec_EmptyName(t *testing.T) {
	s := minimalSpec()
	s.Name = ""
	err := validateSpec(s)
	var invalid *ErrSpecInvalid
	if !errors.As(err, &invalid) || invalid.Field != "Name" {
		t.Fatalf("expected ErrSpecInvalid{Field:Name}, got %v", err)
	}
	if !strings.Contains(invalid.Error(), "Name") {
		t.Fatalf("error message missing field name: %q", invalid.Error())
	}
}

func TestValidateSpec_EmptyVersion(t *testing.T) {
	s := minimalSpec()
	s.Version = ""
	var invalid *ErrSpecInvalid
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "Version" {
		t.Fatalf("expected ErrSpecInvalid{Field:Version}, got %v", err)
	}
}

func TestValidateSpec_NilAwareness(t *testing.T) {
	s := minimalSpec()
	s.Awareness = nil
	var invalid *ErrSpecInvalid
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "Awareness" {
		t.Fatalf("expected ErrSpecInvalid{Field:Awareness}, got %v", err)
	}
}

func TestValidateSpec_NilReasoning(t *testing.T) {
	s := minimalSpec()
	s.Reasoning = nil
	var invalid *ErrSpecInvalid
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "Reasoning" {
		t.Fatalf("expected ErrSpecInvalid{Field:Reasoning}, got %v", err)
	}
}

func TestValidateSpec_DuplicateStateShape(t *testing.T) {
	s := minimalSpec()
	s.States = []StateShape{
		{Name: "x", Factory: func() any { return nil }, Cap: 1},
		{Name: "x", Factory: func() any { return nil }, Cap: 1},
	}
	var dup *ErrDuplicateStateShape
	if err := validateSpec(s); !errors.As(err, &dup) || dup.Shape != "x" {
		t.Fatalf("expected ErrDuplicateStateShape{Shape:x}, got %v", err)
	}
}

func TestValidateSpec_NonPositiveCap(t *testing.T) {
	s := minimalSpec()
	s.States = []StateShape{{Name: "x", Factory: func() any { return nil }, Cap: 0}}
	var invalid *ErrSpecInvalid
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "StateShape.Cap" {
		t.Fatalf("expected ErrSpecInvalid{Field:StateShape.Cap}, got %v", err)
	}
}

func TestValidateSpec_NilStateFactory(t *testing.T) {
	s := minimalSpec()
	s.States = []StateShape{{Name: "x", Cap: 1}}
	var invalid *ErrSpecInvalid
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "StateShape.Factory" {
		t.Fatalf("expected ErrSpecInvalid{Field:StateShape.Factory}, got %v", err)
	}
}

func TestValidateSpec_DuplicateChannelName(t *testing.T) {
	s := minimalSpec()
	channel := ChannelSpec{
		Name:          "c",
		Source:        SubjectSource{Subject: "a"},
		Decode:        func(Message) (any, error) { return nil, nil },
		ExtractEntity: func(any) (Entity, error) { return "e", nil },
	}
	s.Channels = []ChannelSpec{channel, channel}
	var invalid *ErrSpecInvalid
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "ChannelSpec.Name" {
		t.Fatalf("expected ErrSpecInvalid{Field:ChannelSpec.Name}, got %v", err)
	}
}

func TestValidateSpec_MissingSourceKind(t *testing.T) {
	s := minimalSpec()
	s.Channels = []ChannelSpec{{
		Name:          "c",
		Decode:        func(Message) (any, error) { return nil, nil },
		ExtractEntity: func(any) (Entity, error) { return "e", nil },
	}}
	var invalid *ErrSpecInvalid
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "ChannelSpec.Source" {
		t.Fatalf("expected ErrSpecInvalid{Field:ChannelSpec.Source}, got %v", err)
	}
}

func TestValidateSpec_EmptySubjectSource(t *testing.T) {
	s := minimalSpec()
	s.Channels = []ChannelSpec{{
		Name:          "c",
		Source:        SubjectSource{},
		Decode:        func(Message) (any, error) { return nil, nil },
		ExtractEntity: func(any) (Entity, error) { return "e", nil },
	}}
	var invalid *ErrSpecInvalid
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "SubjectSource.Subject" {
		t.Fatalf("expected ErrSpecInvalid{Field:SubjectSource.Subject}, got %v", err)
	}
}

func TestValidateSpec_EmptyStreamSourceFields(t *testing.T) {
	s := minimalSpec()
	s.Channels = []ChannelSpec{{
		Name:          "c",
		Source:        StreamSource{Stream: "S"},
		Decode:        func(Message) (any, error) { return nil, nil },
		ExtractEntity: func(any) (Entity, error) { return "e", nil },
	}}
	var invalid *ErrSpecInvalid
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "StreamSource.FilterSubject" {
		t.Fatalf("expected ErrSpecInvalid{Field:StreamSource.FilterSubject}, got %v", err)
	}

	s.Channels[0].Source = StreamSource{FilterSubject: "x"}
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "StreamSource.Stream" {
		t.Fatalf("expected ErrSpecInvalid{Field:StreamSource.Stream}, got %v", err)
	}
}

func TestValidateSpec_NilDecodeOrExtractor(t *testing.T) {
	s := minimalSpec()
	s.Channels = []ChannelSpec{{
		Name:          "c",
		Source:        SubjectSource{Subject: "x"},
		ExtractEntity: func(any) (Entity, error) { return "e", nil },
	}}
	var invalid *ErrSpecInvalid
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "ChannelSpec.Decode" {
		t.Fatalf("expected ErrSpecInvalid{Field:ChannelSpec.Decode}, got %v", err)
	}

	s.Channels[0].Decode = func(Message) (any, error) { return nil, nil }
	s.Channels[0].ExtractEntity = nil
	if err := validateSpec(s); !errors.As(err, &invalid) || invalid.Field != "ChannelSpec.ExtractEntity" {
		t.Fatalf("expected ErrSpecInvalid{Field:ChannelSpec.ExtractEntity}, got %v", err)
	}
}
