// Package cloudevents provides CloudEvents v1.0.2 serialization and deserialization
// for gopipe messages. It supports both HTTP protocol binding and JSON format.
package cloudevents

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fxsml/gopipe/message"
)

// CloudEvents specification version.
const SpecVersion = "1.0"

// Event represents a CloudEvent structure per CloudEvents v1.0.2 spec.
// It includes required context attributes (id, source, specversion, type),
// optional attributes (subject, time, datacontenttype), and extension attributes.
//
// Per the JSON format spec, data and data_base64 are mutually exclusive:
// - data: for JSON-formatted content or string-encoded content
// - data_base64: for Base64-encoded binary content
type Event struct {
	// Required attributes
	ID          string `json:"id"`
	Source      string `json:"source"`
	SpecVersion string `json:"specversion"`
	Type        string `json:"type"`

	// Optional standard attributes
	Subject         string `json:"subject,omitempty"`
	Time            string `json:"time,omitempty"`
	DataContentType string `json:"datacontenttype,omitempty"`

	// Data payload - one of Data or DataBase64 should be set
	Data       any    `json:"data,omitempty"`
	DataBase64 string `json:"data_base64,omitempty"`

	// Extension attributes (marshaled at top level)
	Extensions map[string]any `json:"-"`
}

// MarshalJSON implements custom JSON marshaling to include extensions at top level.
func (e *Event) MarshalJSON() ([]byte, error) {
	type Alias Event
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	}

	data, err := json.Marshal(aux)
	if err != nil {
		return nil, err
	}

	if len(e.Extensions) == 0 {
		return data, nil
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	for k, v := range e.Extensions {
		m[k] = v
	}

	return json.Marshal(m)
}

// UnmarshalJSON implements custom JSON unmarshaling to extract extensions.
func (e *Event) UnmarshalJSON(data []byte) error {
	type Alias Event
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Extract extensions (all fields not in standard set)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	standardKeys := map[string]bool{
		"id": true, "source": true, "specversion": true, "type": true,
		"subject": true, "time": true, "datacontenttype": true, "data": true,
	}

	e.Extensions = make(map[string]any)
	for k, v := range m {
		if !standardKeys[k] {
			e.Extensions[k] = v
		}
	}

	return nil
}

// FromMessage converts a gopipe message to a CloudEvent.
// The topic is stored as an extension attribute.
// If source is not provided in attributes, defaultSource is used.
func FromMessage(msg *message.Message, topic string, defaultSource string) *Event {
	e := &Event{
		SpecVersion: SpecVersion,
		Extensions:  make(map[string]any),
	}

	// Set required attributes with defaults
	id, _ := msg.Attributes.ID()
	if id == "" {
		id = generateID()
	}
	e.ID = id

	source, _ := msg.Attributes.Source()
	if source == "" {
		source = defaultSource
	}
	e.Source = source

	eventType, _ := msg.Attributes.Type()
	if eventType == "" {
		eventType = "com.gopipe.message"
	}
	e.Type = eventType

	// Set optional standard attributes
	if subject, ok := msg.Attributes.Subject(); ok {
		e.Subject = subject
	}

	if t, ok := msg.Attributes.EventTime(); ok {
		e.Time = t.Format(time.RFC3339Nano)
	}

	if dct, ok := msg.Attributes.DataContentType(); ok {
		e.DataContentType = dct
	}

	// Set data according to CloudEvents JSON format spec:
	// - If datacontenttype ends with +json or is application/json, store as JSON value
	// - Otherwise, use data_base64 for binary content
	if isJSONContentType(e.DataContentType) {
		var jsonData any
		if err := json.Unmarshal(msg.Data, &jsonData); err == nil {
			// Valid JSON - store directly
			e.Data = jsonData
		} else {
			// Invalid JSON but declared as JSON content type - store as string
			e.Data = string(msg.Data)
		}
	} else {
		// Non-JSON content type - use Base64 encoding for binary data
		// Exception: if it's clearly text (utf-8 string), store as data string
		if isLikelyText(msg.Data) {
			e.Data = string(msg.Data)
		} else {
			e.DataBase64 = base64.StdEncoding.EncodeToString(msg.Data)
		}
	}

	// Add gopipe extensions
	if topic != "" {
		e.Extensions["topic"] = topic
	}

	if correlationID, ok := msg.Attributes.CorrelationID(); ok {
		e.Extensions["correlationid"] = correlationID
	}

	if deadline, ok := msg.Attributes.Deadline(); ok {
		e.Extensions["deadline"] = deadline.Format(time.RFC3339Nano)
	}

	// Add other extension attributes (skip standard ones)
	for key, value := range msg.Attributes {
		if !isStandardAttribute(key) {
			e.Extensions[key] = value
		}
	}

	return e
}

// ToMessage converts a CloudEvent to a gopipe message.
// Returns the message and the topic (from extension attribute).
func ToMessage(e *Event) (*message.Message, string, error) {
	attrs := message.Attributes{}

	// Validate and extract required attributes
	if e.ID == "" {
		return nil, "", fmt.Errorf("cloudevents: missing required attribute 'id'")
	}
	attrs[message.AttrID] = e.ID

	if e.Source == "" {
		return nil, "", fmt.Errorf("cloudevents: missing required attribute 'source'")
	}
	attrs[message.AttrSource] = e.Source

	if e.SpecVersion == "" {
		return nil, "", fmt.Errorf("cloudevents: missing required attribute 'specversion'")
	}
	attrs[message.AttrSpecVersion] = e.SpecVersion

	if e.Type == "" {
		return nil, "", fmt.Errorf("cloudevents: missing required attribute 'type'")
	}
	attrs[message.AttrType] = e.Type

	// Extract optional standard attributes
	if e.Subject != "" {
		attrs[message.AttrSubject] = e.Subject
	}

	if e.Time != "" {
		if t, err := time.Parse(time.RFC3339Nano, e.Time); err == nil {
			attrs[message.AttrTime] = t
		}
	}

	if e.DataContentType != "" {
		attrs[message.AttrDataContentType] = e.DataContentType
	}

	// Extract gopipe extension attributes
	topic := ""
	if topicVal, ok := e.Extensions["topic"].(string); ok {
		topic = topicVal
		attrs[message.AttrTopic] = topicVal
	}

	if correlationID, ok := e.Extensions["correlationid"].(string); ok {
		attrs[message.AttrCorrelationID] = correlationID
	}

	if deadlineStr, ok := e.Extensions["deadline"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, deadlineStr); err == nil {
			attrs[message.AttrDeadline] = t
		}
	}

	// Extract other extension attributes
	for key, value := range e.Extensions {
		// Skip already processed extensions
		if key == "topic" || key == "correlationid" || key == "deadline" {
			continue
		}
		attrs[key] = value
	}

	// Extract data (data and data_base64 are mutually exclusive)
	var data []byte
	if e.DataBase64 != "" {
		// Decode Base64 binary data
		var err error
		data, err = base64.StdEncoding.DecodeString(e.DataBase64)
		if err != nil {
			return nil, "", fmt.Errorf("cloudevents: failed to decode data_base64: %w", err)
		}
	} else if e.Data != nil {
		// If datacontenttype is JSON or empty (default), marshal the data to JSON
		// to preserve the JSON encoding. This ensures "first" becomes `"first"` again.
		if isJSONContentType(e.DataContentType) {
			var err error
			data, err = json.Marshal(e.Data)
			if err != nil {
				return nil, "", fmt.Errorf("cloudevents: failed to marshal data: %w", err)
			}
		} else {
			// Non-JSON content type - treat as string
			switch v := e.Data.(type) {
			case string:
				data = []byte(v)
			default:
				// Fallback: marshal to JSON
				var err error
				data, err = json.Marshal(v)
				if err != nil {
					return nil, "", fmt.Errorf("cloudevents: failed to marshal data: %w", err)
				}
			}
		}
	}

	msg := message.New(data, attrs)
	return msg, topic, nil
}

// isStandardAttribute checks if an attribute key is a CloudEvents standard attribute.
func isStandardAttribute(key string) bool {
	switch key {
	case message.AttrID, message.AttrSource, message.AttrSpecVersion, message.AttrType,
		message.AttrSubject, message.AttrTime, message.AttrDataContentType,
		message.AttrTopic, message.AttrCorrelationID, message.AttrDeadline:
		return true
	}
	return false
}

// generateID generates a unique ID for a CloudEvent.
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// isJSONContentType checks if the content type is JSON-based.
// Returns true for application/json or any type ending with +json.
func isJSONContentType(contentType string) bool {
	if contentType == "" {
		return true // Default to JSON if not specified
	}
	ct := strings.ToLower(contentType)
	return ct == "application/json" || strings.HasSuffix(ct, "+json") || strings.Contains(ct, "application/json")
}

// isLikelyText checks if data is likely printable text (UTF-8).
// This is a heuristic to decide whether to use data or data_base64.
func isLikelyText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	// Check if all bytes are printable ASCII or valid UTF-8
	for _, b := range data {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			return false // Control character
		}
		if b == 0x7F {
			return false // DEL character
		}
	}
	return true
}
