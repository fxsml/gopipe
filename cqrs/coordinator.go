package cqrs

import (
	"context"

	"github.com/fxsml/gopipe/message"
)

// SagaCoordinator handles events and returns commands for workflow coordination.
type SagaCoordinator interface {
	OnEvent(ctx context.Context, msg *message.Message) ([]*message.Message, error)
}
