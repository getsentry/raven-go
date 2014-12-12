package raven

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"time"
)

var hostname string

type Tag struct {
	Key   string
	Value string
}

func (tag *Tag) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]string{tag.Key, tag.Value})
}

// http://sentry.readthedocs.org/en/latest/developer/client/index.html#building-the-json-packet
type Event struct {
	// Required
	Message string `json:"message"`

	// Required, set automatically by Client.Capture via Event.FillDefaults if blank.
	EventID   string    `json:"event_id"`
	Project   string    `json:"project"`
	Timestamp Timestamp `json:"timestamp"`
	Level     Severity  `json:"level"`
	Logger    string    `json:"logger"`

	// Optional
	Platform   string                 `json:"platform,omitempty"`
	Culprit    string                 `json:"culprit,omitempty"`
	Tags       []Tag                  `json:"tags,omitempty"`
	ServerName string                 `json:"server_name,omitempty"`
	Modules    []map[string]string    `json:"modules,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`

	Interfaces []Interface `json:"-"`
}

func (event *Event) Merge(otherEvent *Event) *Event {
	// Override
	if otherEvent.Message != "" {
		event.Message = otherEvent.Message
	}
	if otherEvent.EventID != "" {
		event.EventID = otherEvent.EventID
	}
	if otherEvent.Project != "" {
		event.Project = otherEvent.Project
	}
	if !time.Time(otherEvent.Timestamp).IsZero() {
		event.Timestamp = otherEvent.Timestamp
	}
	if otherEvent.Level != 0 {
		event.Level = otherEvent.Level
	}
	if otherEvent.Logger != "" {
		event.Logger = otherEvent.Logger
	}
	if otherEvent.Platform != "" {
		event.Platform = otherEvent.Platform
	}
	if otherEvent.Culprit != "" {
		event.Culprit = otherEvent.Culprit
	}
	if otherEvent.ServerName != "" {
		event.ServerName = otherEvent.ServerName
	}

	// Append
	event.Tags = append(event.Tags, otherEvent.Tags...)
	event.Modules = append(event.Modules, otherEvent.Modules...)
	event.Interfaces = append(event.Interfaces, otherEvent.Interfaces...)

	// Merge
	for k, v := range otherEvent.Extra {
		event.Extra[k] = v
	}

	return event
}

func (event *Event) FillDefaults(project string) error {
	// Required fields.
	if event.EventID == "" {
		uuid4, err := uuid()
		if err != nil {
			return err
		}
		event.EventID = uuid4
	}

	if event.Project == "" {
		event.Project = project
	}
	if time.Time(event.Timestamp).IsZero() {
		event.Timestamp = Timestamp(time.Now())
	}
	if event.Level == 0 {
		event.Level = ERROR
	}
	if event.Logger == "" {
		event.Logger = "root"
	}

	// Nice to have, we can help.
	if event.ServerName == "" {
		event.ServerName = hostname
	}
	if event.Culprit == "" {
		for _, inter := range event.Interfaces {
			if c, ok := inter.(Culpriter); ok {
				event.Culprit = c.Culprit()
				if event.Culprit != "" {
					break
				}
			}
		}
	}

	return nil
}

func (event *Event) JSON() []byte {
	eventJSON, _ := json.Marshal(event)

	interfaces := make(map[string]Interface, len(event.Interfaces))
	for _, inter := range event.Interfaces {
		interfaces[inter.Class()] = inter
	}

	if len(interfaces) > 0 {
		interfaceJSON, _ := json.Marshal(interfaces)
		eventJSON[len(eventJSON)-1] = ','
		eventJSON = append(eventJSON, interfaceJSON[1:]...)
	}

	return eventJSON
}

func init() {
	hostname, _ = os.Hostname()
}

func uuid() (string, error) {
	id := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		return "", err
	}

	id[6] &= 0x0F // clear version
	id[6] |= 0x40 // set version to 4 (random uuid)
	id[8] &= 0x3F // clear variant
	id[8] |= 0x80 // set to IETF variant

	return hex.EncodeToString(id), nil
}

func (event *Event) serialize() (r io.Reader, contentType string) {
	eventJSON := event.JSON()

	// Only deflate/base64 the event if it is bigger than 1KB, as there is
	// overhead.
	if len(eventJSON) > 1000 {
		buf := &bytes.Buffer{}
		b64 := base64.NewEncoder(base64.StdEncoding, buf)
		deflate, _ := zlib.NewWriterLevel(b64, zlib.BestCompression)
		deflate.Write(eventJSON)
		deflate.Close()
		b64.Close()

		return buf, "application/octet-stream"
	}

	return bytes.NewReader(eventJSON), "application/json"
}
