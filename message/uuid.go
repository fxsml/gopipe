package message

import (
	"crypto/rand"
	"encoding/hex"
)

// IDGenerator generates unique message IDs.
// The default implementation generates RFC 4122 UUID v4 strings.
type IDGenerator func() string

// DefaultIDGenerator is used by Builder and other components to generate IDs.
// Replace with uuid.NewString from github.com/google/uuid for production
// systems requiring pooled randomness for high throughput.
var DefaultIDGenerator IDGenerator = NewID

// NewID generates a RFC 4122 compliant UUID v4 string.
// Returns format: "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx"
//
// Uses crypto/rand for cryptographically secure randomness.
// This is a zero-dependency implementation suitable for CloudEvents
// id attribute generation. For high-throughput production systems,
// consider using github.com/google/uuid which implements pooled
// randomness for better performance.
//
// The function has the same signature as uuid.NewString from
// github.com/google/uuid, allowing easy migration:
//
//	// Switch to google/uuid
//	import "github.com/google/uuid"
//	message.DefaultIDGenerator = uuid.NewString
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns error

	// Version 4: set bits 12-15 of time_hi_and_version to 0100
	b[6] = (b[6] & 0x0f) | 0x40
	// Variant RFC 4122: set bits 6-7 of clock_seq_hi_and_reserved to 10
	b[8] = (b[8] & 0x3f) | 0x80

	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])

	return string(buf[:])
}
