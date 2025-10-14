package middleware_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/middleware"
)

func TestCancel_AppliesAdditionalCancelFunction(t *testing.T) {
	var (
		processorCancelCalled bool
		customCancelCalled    bool
		receivedInput         string
		receivedError         error
		testError             = errors.New("test error")
		testInput             = "test input"
	)

	baseProcessor := gopipe.NewProcessor(
		func(ctx context.Context, in string) (string, error) {
			return in + "-processed", nil
		},
		func(in string, err error) {
			processorCancelCalled = true
		},
	)

	customCancel := func(in string, err error) {
		customCancelCalled = true
		receivedInput = in
		receivedError = err
	}

	processor := middleware.UseCancel[string, string](customCancel)(baseProcessor)

	processor.Cancel(testInput, testError)
	if !processorCancelCalled {
		t.Error("The base processor's Cancel function was not called")
	}

	if !customCancelCalled {
		t.Error("The custom Cancel function was not called")
	}

	if receivedInput != testInput {
		t.Errorf("Expected input %q but got %q", testInput, receivedInput)
	}

	if receivedError != testError {
		t.Errorf("Expected error %v but got %v", testError, receivedError)
	}
}

func TestCancel_ProcessBehaviorUnchanged(t *testing.T) {
	expectedOutput := "input-processed"

	baseProcessor := gopipe.NewProcessor(
		func(ctx context.Context, in string) (string, error) {
			return expectedOutput, nil
		},
		func(in string, err error) {},
	)

	cancelCalledDuringProcess := false
	customCancel := func(in string, err error) {
		cancelCalledDuringProcess = true
	}

	processor := middleware.UseCancel[string, string](customCancel)(baseProcessor)

	output, err := processor.Process(context.Background(), "input")
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}

	if output != expectedOutput {
		t.Errorf("Expected output %q but got %q", expectedOutput, output)
	}

	if cancelCalledDuringProcess {
		t.Error("The custom cancel function was incorrectly called during Process")
	}
}
