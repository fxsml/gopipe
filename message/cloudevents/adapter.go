package cloudevents

import (
	"encoding/json"
	"fmt"
	"maps"
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

	attrs[message.AttrID] = e.ID()
	attrs[message.AttrSpecVersion] = e.SpecVersion()
	attrs[message.AttrType] = e.Type()
	attrs[message.AttrSource] = e.Source()
	if dct := e.DataContentType(); dct != "" {
		attrs[message.AttrDataContentType] = dct
	}
	if ds := e.DataSchema(); ds != "" {
		attrs[message.AttrDataSchema] = ds
	}
	if subj := e.Subject(); subj != "" {
		attrs[message.AttrSubject] = subj
	}
	if t := e.Time(); !t.IsZero() {
		attrs[message.AttrTime] = t.UTC().Format(time.RFC3339)
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
		isNonEmptyString := isString && s != ""
		switch k {
		case message.AttrID:
			if isNonEmptyString {
				e.SetID(s)
			}
		case message.AttrType:
			if isNonEmptyString {
				e.SetType(s)
			}
		case message.AttrSource:
			if isNonEmptyString {
				e.SetSource(s)
			}
		case message.AttrSpecVersion:
			if isNonEmptyString {
				e.SetSpecVersion(s)
			}
		case message.AttrTime:
			if isTime, ok := v.(time.Time); ok && !isTime.IsZero() {
				e.SetTime(isTime)
			}
			if isNonEmptyString {
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					e.SetTime(t)
				}
			}
		case message.AttrDataContentType:
			if isNonEmptyString {
				ct = s
				e.SetDataContentType(s)
			}
		case message.AttrDataSchema:
			if isNonEmptyString {
				e.SetDataSchema(s)
			}
		case message.AttrSubject:
			if isNonEmptyString {
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

	maps.Copy(attrs, e.Extensions())

	attrs[message.AttrID] = e.ID()
	attrs[message.AttrSpecVersion] = e.SpecVersion()
	attrs[message.AttrType] = e.Type()
	attrs[message.AttrSource] = e.Source()
	if dct := e.DataContentType(); dct != "" {
		attrs[message.AttrDataContentType] = dct
	}
	if ds := e.DataSchema(); ds != "" {
		attrs[message.AttrDataSchema] = ds
	}
	if subj := e.Subject(); subj != "" {
		attrs[message.AttrSubject] = subj
	}
	if t := e.Time(); !t.IsZero() {
		attrs[message.AttrTime] = t.UTC().Format(time.RFC3339)
	}

	return attrs
}
