package cloudevents

import (
	"encoding/json"
	"fmt"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/fxsml/gopipe/message"
)

// FromCloudEvent converts a cloudevents.Event into a RawMessage.
// Copies standard attributes and extensions, and returns event data as []byte.
// The acking parameter is used to bridge CloudEvents acknowledgment to gopipe.
func FromCloudEvent(e *cloudevents.Event, acking *message.Acking) (*message.RawMessage, error) {
	attrs := message.Attributes{}
	if e == nil {
		return nil, fmt.Errorf("nil event")
	}

	attrs["id"] = e.ID()
	attrs["specversion"] = e.SpecVersion()
	attrs["type"] = e.Type()
	attrs["source"] = e.Source()
	if dct := e.DataContentType(); dct != "" {
		attrs["datacontenttype"] = dct
	}
	if ds := e.DataSchema(); ds != "" {
		attrs["dataschema"] = ds
	}
	if subj := e.Subject(); subj != "" {
		attrs["subject"] = subj
	}
	if t := e.Time(); !t.IsZero() {
		attrs["time"] = t.UTC().Format(time.RFC3339)
	}

	for k, v := range e.Extensions() {
		attrs[k] = v
	}

	var data []byte
	if b := e.Data(); len(b) > 0 {
		data = append([]byte(nil), b...)
	}

	return message.NewRaw(data, attrs, acking), nil
}

// ToCloudEvent converts a RawMessage into a cloudevents.Event.
// If datacontenttype is not set in attributes, it is omitted (allowed for binary data).
func ToCloudEvent(msg *message.RawMessage) (*cloudevents.Event, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil RawMessage")
	}

	e := cloudevents.NewEvent()

	if v, ok := msg.Attributes["id"].(string); ok && v != "" {
		e.SetID(v)
	}
	if v, ok := msg.Attributes["type"].(string); ok && v != "" {
		e.SetType(v)
	}
	if v, ok := msg.Attributes["source"].(string); ok && v != "" {
		e.SetSource(v)
	}
	if v, ok := msg.Attributes["specversion"].(string); ok && v != "" {
		e.SetSpecVersion(v)
	}
	if v, ok := msg.Attributes["time"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			e.SetTime(t)
		}
	}

	// set content type from attributes if present (omit if not set - allowed for binary)
	var ct string
	if v, ok := msg.Attributes["datacontenttype"].(string); ok && v != "" {
		ct = v
		e.SetDataContentType(ct)
	}
	if v, ok := msg.Attributes["dataschema"].(string); ok && v != "" {
		e.SetDataSchema(v)
	}
	if v, ok := msg.Attributes["subject"].(string); ok && v != "" {
		e.SetSubject(v)
	}

	// copy non-standard attributes as extensions
	for k, v := range msg.Attributes {
		switch k {
		case "id", "type", "source", "specversion", "time", "datacontenttype", "dataschema", "subject":
			continue
		default:
			e.SetExtension(k, v)
		}
	}

	// RawMessage.Data is []byte (possibly nil)
	if data := msg.Data; data != nil {
		var err error
		if ct == "application/json" && json.Valid(data) {
			// preserve structured JSON
			err = e.SetData(ct, json.RawMessage(data))
		} else {
			// set raw bytes (SDK accepts []byte)
			err = e.SetData(ct, data)
		}
		if err != nil {
			return nil, fmt.Errorf("set data: %w", err)
		}
	}

	return &e, nil
}

// extractAttributes extracts CloudEvents attributes into a message.Attributes map.
func extractAttributes(e *cloudevents.Event) message.Attributes {
	attrs := message.Attributes{}

	attrs["id"] = e.ID()
	attrs["specversion"] = e.SpecVersion()
	attrs["type"] = e.Type()
	attrs["source"] = e.Source()
	if dct := e.DataContentType(); dct != "" {
		attrs["datacontenttype"] = dct
	}
	if ds := e.DataSchema(); ds != "" {
		attrs["dataschema"] = ds
	}
	if subj := e.Subject(); subj != "" {
		attrs["subject"] = subj
	}
	if t := e.Time(); !t.IsZero() {
		attrs["time"] = t.UTC().Format(time.RFC3339)
	}

	for k, v := range e.Extensions() {
		attrs[k] = v
	}

	return attrs
}
