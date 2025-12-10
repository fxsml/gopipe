package middleware

import (
	"context"
	"testing"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

func TestMessageType_SetsTypeFromData(t *testing.T) {
	innerProc := gopipe.NewProcessor(
		func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			outMsg := message.New([]byte(`{"order":"123"}`), message.Attributes{})
			return []*message.Message{outMsg}, nil
		},
		func(m *message.Message, err error) {},
	)

	middleware := MessageType()
	wrappedProc := middleware(innerProc)

	results, err := wrappedProc.Process(context.Background(), &message.Message{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgType, ok := results[0].Attributes.Type()
	if !ok || msgType == "" {
		t.Error("expected type to be set")
	}
}

func TestMessageType_AlwaysOverwrites(t *testing.T) {
	innerProc := gopipe.NewProcessor(
		func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			outMsg := message.New([]byte(`[1,2,3]`), message.Attributes{
				message.AttrType: "existing.type",
			})
			return []*message.Message{outMsg}, nil
		},
		func(m *message.Message, err error) {},
	)

	middleware := MessageType()
	wrappedProc := middleware(innerProc)

	results, err := wrappedProc.Process(context.Background(), &message.Message{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgType, _ := results[0].Attributes.Type()
	if msgType != "slice" {
		t.Errorf("expected type to be 'slice', got %q", msgType)
	}
}

func TestMessageType_HandlesInvalidJSON(t *testing.T) {
	innerProc := gopipe.NewProcessor(
		func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			outMsg := message.New([]byte("not json"), message.Attributes{})
			return []*message.Message{outMsg}, nil
		},
		func(m *message.Message, err error) {},
	)

	middleware := MessageType()
	wrappedProc := middleware(innerProc)

	results, err := wrappedProc.Process(context.Background(), &message.Message{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := results[0].Attributes.Type(); ok {
		t.Error("expected type not to be set for invalid JSON")
	}
}

func TestMessageType_HandlesMultipleMessages(t *testing.T) {
	innerProc := gopipe.NewProcessor(
		func(ctx context.Context, m *message.Message) ([]*message.Message, error) {
			msg1 := message.New([]byte(`{"key":"value"}`), message.Attributes{})
			msg2 := message.New([]byte(`[1,2,3]`), message.Attributes{})
			msg3 := message.New([]byte(`"text"`), message.Attributes{})
			return []*message.Message{msg1, msg2, msg3}, nil
		},
		func(m *message.Message, err error) {},
	)

	middleware := MessageType()
	wrappedProc := middleware(innerProc)

	results, err := wrappedProc.Process(context.Background(), &message.Message{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"map", "slice", "string"}
	for i, result := range results {
		msgType, ok := result.Attributes.Type()
		if !ok {
			t.Errorf("message %d: expected type to be set", i)
			continue
		}
		if msgType != expected[i] {
			t.Errorf("message %d: expected type %q, got %q", i, expected[i], msgType)
		}
	}
}
