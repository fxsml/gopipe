package middleware

import (
	"context"
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

	// Create a process func that checks for metadata in its context
	var receivedMetadata Metadata
	processFunc := func(ctx context.Context, in string) ([]string, error) {
		// Extract and store metadata from context for verification
		receivedMetadata = MetadataFromContext(ctx)
		return []string{in + "-processed"}, nil
	}

	// Apply the metadata middleware
	metadataMw := MetadataProvider[string, string](metadataFn)
	wrappedFunc := metadataMw(processFunc)

	// Process an item
	input := "test-input"
	results, err := wrappedFunc(context.Background(), input)
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

func TestMetadataArgs_ConvertsToKeyValuePairs(t *testing.T) {
	// Create metadata with various types of values
	metadata := Metadata{
		"string":  "value",
		"integer": 42,
		"boolean": true,
	}

	// Convert to args
	args := metadata.Args()

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
