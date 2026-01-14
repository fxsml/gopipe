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
	var ct string

	for k, v := range msg.Attributes {
		s, isString := v.(string)
		switch k {
		case "id":
			if isString && s != "" {
				e.SetID(s)
			}
		case "type":
			if isString && s != "" {
				e.SetType(s)
			}
		case "source":
			if isString && s != "" {
				e.SetSource(s)
			}
		case "specversion":
			if isString && s != "" {
				e.SetSpecVersion(s)
			}
		case "time":
			if isString && s != "" {
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					e.SetTime(t)
				}
			}
		case "datacontenttype":
			if isString && s != "" {
				ct = s
				e.SetDataContentType(s)
			}
		case "dataschema":
			if isString && s != "" {
				e.SetDataSchema(s)
			}
		case "subject":
			if isString && s != "" {
				e.SetSubject(s)
			}
		default:
			e.SetExtension(k, v)
		}
	}

	if data := msg.Data; data != nil {
		var err error
		if ct == "application/json" && json.Valid(data) {
			err = e.SetData(ct, json.RawMessage(data))
		} else {
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
