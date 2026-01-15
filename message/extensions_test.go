package message

import "testing"

func TestIsRequiredAttr(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{AttrID, true},
		{AttrType, true},
		{AttrSource, true},
		{AttrSpecVersion, true},
		{AttrSubject, false},
		{AttrTime, false},
		{AttrCorrelationID, false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := IsRequiredAttr(tt.key); got != tt.want {
				t.Errorf("IsRequiredAttr(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestIsOptionalAttr(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{AttrSubject, true},
		{AttrTime, true},
		{AttrDataContentType, true},
		{AttrDataSchema, true},
		{AttrID, false},
		{AttrType, false},
		{AttrCorrelationID, false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := IsOptionalAttr(tt.key); got != tt.want {
				t.Errorf("IsOptionalAttr(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestIsExtensionAttr(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{AttrCorrelationID, true},
		{AttrExpiryTime, true},
		{"customext", true},
		{AttrID, false},
		{AttrType, false},
		{AttrSource, false},
		{AttrSpecVersion, false},
		{AttrSubject, false},
		{AttrTime, false},
		{AttrDataContentType, false},
		{AttrDataSchema, false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := IsExtensionAttr(tt.key); got != tt.want {
				t.Errorf("IsExtensionAttr(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestAttrPredicatesCompleteness(t *testing.T) {
	// Every CloudEvents attribute should be in exactly one category
	allAttrs := []string{
		AttrID, AttrType, AttrSource, AttrSpecVersion,
		AttrSubject, AttrTime, AttrDataContentType, AttrDataSchema,
	}

	for _, attr := range allAttrs {
		required := IsRequiredAttr(attr)
		optional := IsOptionalAttr(attr)
		extension := IsExtensionAttr(attr)

		count := 0
		if required {
			count++
		}
		if optional {
			count++
		}
		if extension {
			count++
		}

		if count != 1 {
			t.Errorf("attribute %q is in %d categories (should be exactly 1)", attr, count)
		}
	}
}
