package message_test

import (
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/fxsml/gopipe/message"
)

func TestNewID_Format(t *testing.T) {
	t.Parallel()

	id := message.NewID()

	// UUID v4 format: 8-4-4-4-12 hex chars with version 4 and variant 8/9/a/b
	pattern := `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
	matched, err := regexp.MatchString(pattern, id)
	if err != nil {
		t.Fatalf("regexp error: %v", err)
	}
	if !matched {
		t.Errorf("UUID format invalid: %s", id)
	}
}

func TestNewID_Length(t *testing.T) {
	t.Parallel()

	id := message.NewID()

	// UUID string is always 36 characters (32 hex + 4 hyphens)
	if len(id) != 36 {
		t.Errorf("expected length 36, got %d: %s", len(id), id)
	}
}

func TestNewID_Version4(t *testing.T) {
	t.Parallel()

	// Generate multiple UUIDs to ensure consistency
	for i := 0; i < 100; i++ {
		id := message.NewID()
		// Position 14 should be '4' (version)
		if id[14] != '4' {
			t.Errorf("expected version 4 at position 14, got %c in %s", id[14], id)
		}
	}
}

func TestNewID_VariantRFC4122(t *testing.T) {
	t.Parallel()

	// Generate multiple UUIDs to ensure consistency
	for i := 0; i < 100; i++ {
		id := message.NewID()
		// Position 19 should be 8, 9, a, or b (RFC 4122 variant)
		variant := id[19]
		if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
			t.Errorf("expected variant 8/9/a/b at position 19, got %c in %s", variant, id)
		}
	}
}

func TestNewID_Uniqueness(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	count := 10000

	for i := 0; i < count; i++ {
		id := message.NewID()
		if seen[id] {
			t.Errorf("duplicate UUID detected: %s", id)
		}
		seen[id] = true
	}

	if len(seen) != count {
		t.Errorf("expected %d unique UUIDs, got %d", count, len(seen))
	}
}

func TestNewID_HyphenPositions(t *testing.T) {
	t.Parallel()

	id := message.NewID()

	// Hyphens at positions 8, 13, 18, 23
	hyphenPositions := []int{8, 13, 18, 23}
	for _, pos := range hyphenPositions {
		if id[pos] != '-' {
			t.Errorf("expected hyphen at position %d, got %c in %s", pos, id[pos], id)
		}
	}
}

func TestNewID_LowercaseHex(t *testing.T) {
	t.Parallel()

	id := message.NewID()

	// All hex characters should be lowercase
	for i, c := range id {
		if c == '-' {
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("invalid character %c at position %d in %s", c, i, id)
		}
	}
}

func TestNewID_NoUppercase(t *testing.T) {
	t.Parallel()

	// Generate many UUIDs and ensure none contain uppercase
	for i := 0; i < 1000; i++ {
		id := message.NewID()
		if strings.ToLower(id) != id {
			t.Errorf("UUID contains uppercase: %s", id)
		}
	}
}

func TestNewID_SegmentLengths(t *testing.T) {
	t.Parallel()

	id := message.NewID()
	segments := strings.Split(id, "-")

	if len(segments) != 5 {
		t.Fatalf("expected 5 segments, got %d in %s", len(segments), id)
	}

	expectedLengths := []int{8, 4, 4, 4, 12}
	for i, seg := range segments {
		if len(seg) != expectedLengths[i] {
			t.Errorf("segment %d: expected length %d, got %d in %s",
				i, expectedLengths[i], len(seg), id)
		}
	}
}

func TestDefaultIDGenerator(t *testing.T) {
	t.Parallel()

	// DefaultIDGenerator should be set to NewID
	if message.DefaultIDGenerator == nil {
		t.Error("DefaultIDGenerator should not be nil")
	}

	id := message.DefaultIDGenerator()
	if len(id) != 36 {
		t.Errorf("DefaultIDGenerator returned invalid UUID: %s", id)
	}
}

func TestDefaultIDGenerator_Replaceable(t *testing.T) {
	// Save original
	original := message.DefaultIDGenerator
	defer func() { message.DefaultIDGenerator = original }()

	// Replace with custom generator
	customID := "custom-test-id-12345"
	message.DefaultIDGenerator = func() string {
		return customID
	}

	if message.DefaultIDGenerator() != customID {
		t.Error("DefaultIDGenerator should be replaceable")
	}
}

func TestNewID_ConcurrentSafety(t *testing.T) {
	t.Parallel()

	const goroutines = 100
	const idsPerGoroutine = 100

	var wg sync.WaitGroup
	results := make(chan string, goroutines*idsPerGoroutine)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				results <- message.NewID()
			}
		}()
	}

	wg.Wait()
	close(results)

	// Collect all IDs and check for duplicates
	seen := make(map[string]bool)
	for id := range results {
		if seen[id] {
			t.Errorf("duplicate UUID in concurrent generation: %s", id)
		}
		seen[id] = true
	}

	expected := goroutines * idsPerGoroutine
	if len(seen) != expected {
		t.Errorf("expected %d unique UUIDs, got %d", expected, len(seen))
	}
}

func TestNewID_ValidHexInEachSegment(t *testing.T) {
	t.Parallel()

	id := message.NewID()
	segments := strings.Split(id, "-")

	hexPattern := regexp.MustCompile(`^[0-9a-f]+$`)
	for i, seg := range segments {
		if !hexPattern.MatchString(seg) {
			t.Errorf("segment %d contains invalid hex: %s in %s", i, seg, id)
		}
	}
}

func TestNewID_Randomness(t *testing.T) {
	t.Parallel()

	// Generate UUIDs and check that different parts vary
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = message.NewID()
	}

	// Check that the first segment varies (should be random)
	firstSegments := make(map[string]bool)
	for _, id := range ids {
		seg := strings.Split(id, "-")[0]
		firstSegments[seg] = true
	}

	// With 100 random UUIDs, first segments should be highly varied
	if len(firstSegments) < 90 {
		t.Errorf("first segments lack randomness: only %d unique values in 100 UUIDs",
			len(firstSegments))
	}
}

func TestNewID_VersionAndVariantBits(t *testing.T) {
	t.Parallel()

	for i := 0; i < 100; i++ {
		id := message.NewID()

		// Parse the UUID to check specific bits
		// Remove hyphens
		hex := strings.ReplaceAll(id, "-", "")

		// Version bits are at position 12-15 (nibble 12, which is byte 6 high nibble)
		// In the hex string, this is character 12 (0-indexed)
		versionChar := hex[12]
		if versionChar != '4' {
			t.Errorf("version nibble should be 4, got %c in %s", versionChar, id)
		}

		// Variant bits are at position 16-17 (nibble 16, which is byte 8 high nibble)
		// In the hex string, this is character 16 (0-indexed)
		variantChar := hex[16]
		// Should be 8, 9, a, or b (binary 10xx)
		if variantChar != '8' && variantChar != '9' && variantChar != 'a' && variantChar != 'b' {
			t.Errorf("variant nibble should be 8/9/a/b, got %c in %s", variantChar, id)
		}
	}
}

func TestIDGenerator_Type(t *testing.T) {
	t.Parallel()

	// Verify IDGenerator is a function type
	var gen message.IDGenerator = message.NewID

	id := gen()
	if len(id) != 36 {
		t.Errorf("IDGenerator returned invalid UUID: %s", id)
	}
}

func TestNewID_NotEmpty(t *testing.T) {
	t.Parallel()

	for i := 0; i < 100; i++ {
		id := message.NewID()
		if id == "" {
			t.Error("NewID returned empty string")
		}
		if id == "00000000-0000-0000-0000-000000000000" {
			t.Error("NewID returned nil UUID")
		}
	}
}

func BenchmarkNewID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = message.NewID()
	}
}

func BenchmarkNewID_Parallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = message.NewID()
		}
	})
}

func BenchmarkNewID_Allocs(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = message.NewID()
	}
}
