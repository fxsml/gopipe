package pipe

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMetadata_AttachesToContext(t *testing.T) {
	// Create metadata provider function
	metadataFn := func(in string) Metadata {
		return Metadata{
			"request_id": "12345",
			"input_len":  len(in),
		}
	}

	// Create a processor that checks for metadata in its context
	var receivedMetadata Metadata
	baseProcessor := NewProcessor(
		func(ctx context.Context, in string) ([]string, error) {
			// Extract and store metadata from context for verification
			receivedMetadata = MetadataFromContext(ctx)
			return []string{in + "-processed"}, nil
		},
		func(in string, err error) {},
	)

	// Apply the metadata middleware
	processor := useMetadata[string, string](metadataFn)(baseProcessor)

	// Process an item
	input := "test-input"
	results, err := processor.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify processing worked normally
	if len(results) != 1 || results[0] != "test-input-processed" {
		t.Errorf("Expected [test-input-processed], got %v", results)
	}

	// Verify metadata was properly attached to context
	expectedMetadata := Metadata{
		"request_id": "12345",
		"input_len":  10, // len("test-input")
	}

	if !reflect.DeepEqual(receivedMetadata, expectedMetadata) {
		t.Errorf("Expected metadata %v, got %v", expectedMetadata, receivedMetadata)
	}
}

func TestMetadata_AttachesToError(t *testing.T) {
	testError := errors.New("processing error")

	// Create metadata provider function
	metadataFn := func(in string) Metadata {
		return Metadata{
			"request_id": "12345",
			"input_len":  len(in),
		}
	}

	// Create a processor that captures the cancel call
	var cancelCalled bool
	var cancelError error
	baseProcessor := NewProcessor(
		func(ctx context.Context, in string) ([]string, error) {
			return []string{in}, nil
		},
		func(in string, err error) {
			cancelCalled = true
			cancelError = err
		},
	)

	// Apply the metadata middleware
	processor := useMetadata[string, string](metadataFn)(baseProcessor)

	// Call Cancel directly to test metadata attachment
	input := "test-input"
	processor.Cancel(input, testError)

	// Verify Cancel was called
	if !cancelCalled {
		t.Fatal("Expected Cancel to be called")
	}

	// Verify metadata was attached to the error
	metadata := MetadataFromError(cancelError)
	if metadata == nil {
		t.Fatal("Expected metadata to be attached to error, got nil")
	}

	// Verify metadata content
	expectedMetadata := Metadata{
		"request_id": "12345",
		"input_len":  10, // len("test-input")
	}

	if !reflect.DeepEqual(metadata, expectedMetadata) {
		t.Errorf("Expected metadata %v, got %v", expectedMetadata, metadata)
	}

	// Verify the original error can still be unwrapped
	if !errors.Is(cancelError, testError) {
		t.Errorf("Expected unwrapped error to be %v, got different error", testError)
	}
}

func TestMetadata_ErrorPassthrough(t *testing.T) {
	// Create metadata provider function with tracking
	metadataCalls := 0
	metadataFn := func(in string) Metadata {
		metadataCalls++
		return Metadata{
			"request_id": "12345",
			"input_len":  len(in),
		}
	}

	// Create a processor that just passes errors to Cancel
	var cancelCalled bool
	var receivedErr error
	baseProcessor := NewProcessor(
		func(ctx context.Context, in string) ([]string, error) {
			return []string{in + "-processed"}, nil
		},
		func(in string, err error) {
			cancelCalled = true
			receivedErr = err
		},
	)

	// Apply the metadata middleware
	processor := useMetadata[string, string](metadataFn)(baseProcessor)

	// Create a simple error (not a cancel error)
	testError := errors.New("test error")

	// Call Cancel with a regular error
	inputStr := "test-input"
	processor.Cancel(inputStr, testError)

	// Verify that Cancel was called
	if !cancelCalled {
		t.Fatal("Expected Cancel to be called, but it wasn't")
	}

	// Verify that metadata was called once
	if metadataCalls != 1 {
		t.Errorf("Expected metadataFn to be called once, got %d calls", metadataCalls)
	}

	// Verify the original error can still be unwrapped
	if !errors.Is(receivedErr, testError) {
		t.Errorf("Expected unwrapped error to be %v, got different error", testError)
	}

	// Verify metadata was attached to the error
	metadata := MetadataFromError(receivedErr)
	if metadata == nil {
		t.Fatal("Expected metadata to be attached to error, got nil")
	}

	// Verify metadata content
	expectedMetadata := Metadata{
		"request_id": "12345",
		"input_len":  10, // len("test-input")
	}

	if !reflect.DeepEqual(metadata, expectedMetadata) {
		t.Errorf("Expected metadata %v, got %v", expectedMetadata, metadata)
	}
}

func TestMetadataArgs_ConvertsToKeyValuePairs(t *testing.T) {
	// Create metadata with various types of values
	metadata := Metadata{
		"string":  "value",
		"integer": 42,
		"boolean": true,
	}

	// Convert to args
	args := metadata.args()

	// The result should be a flattened key-value array
	if len(args) != 6 {
		t.Errorf("Expected 6 items in args (3 keys + 3 values), got %d", len(args))
	}

	// Create a map to check the key-value pairs
	pairs := make(map[string]interface{})
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			key, ok := args[i].(string)
			if !ok {
				t.Errorf("Expected string key at position %d, got %T", i, args[i])
				continue
			}
			pairs[key] = args[i+1]
		}
	}

	// Check that all values are preserved
	if pairs["string"] != "value" {
		t.Errorf(`Expected pairs["string"] = "value", got %v`, pairs["string"])
	}
	if pairs["integer"] != 42 {
		t.Errorf(`Expected pairs["integer"] = 42, got %v`, pairs["integer"])
	}
	if pairs["boolean"] != true {
		t.Errorf(`Expected pairs["boolean"] = true, got %v`, pairs["boolean"])
	}
}

func TestMetadataFromContext_HandlesNil(t *testing.T) {
	// Using context.TODO() instead of nil context
	metadata := MetadataFromContext(context.TODO())
	if metadata != nil {
		t.Errorf("Expected nil metadata from empty context, got %v", metadata)
	}

	// Context without metadata should return nil metadata
	metadata = MetadataFromContext(context.Background())
	if metadata != nil {
		t.Errorf("Expected nil metadata from context without metadata, got %v", metadata)
	}
}

func TestMetadataFromError_HandlesNil(t *testing.T) {
	// Nil error should return nil metadata
	metadata := MetadataFromError(nil)
	if metadata != nil {
		t.Errorf("Expected nil metadata from nil error, got %v", metadata)
	}

	// Error without metadata should return nil metadata
	metadata2 := MetadataFromError(errors.New("plain error"))
	if metadata2 != nil {
		t.Errorf("Expected nil metadata from error without metadata, got %v", metadata2)
	}
}
