package raven

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"time"
)

var hostname string

type Tag struct {
	Key   string
	Value string
}

type Tags []Tag

func (tag *Tag) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]string{tag.Key, tag.Value})
}

func (t *Tag) UnmarshalJSON(data []byte) error {
	var tag [2]string
	if err := json.Unmarshal(data, &tag); err != nil {
		return err
	}
	*t = Tag{tag[0], tag[1]}
	return nil
}

func (t *Tags) UnmarshalJSON(data []byte) error {
	var tags []Tag

	switch data[0] {
	case '[':
		// Unmarshal into []Tag
		if err := json.Unmarshal(data, &tags); err != nil {
			return err
		}
	case '{':
		// Unmarshal into map[string]string
		tagMap := make(map[string]string)
		if err := json.Unmarshal(data, &tagMap); err != nil {
			return err
		}

		// Convert to []Tag
		for k, v := range tagMap {
			tags = append(tags, Tag{k, v})
		}
	default:
		return errors.New("raven: unable to unmarshal JSON")
	}

	*t = tags
	return nil
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
	Tags       Tags                   `json:"tags,omitempty"`
	ServerName string                 `json:"server_name,omitempty"`
	Modules    []map[string]string    `json:"modules,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`

	Interfaces []Interface `json:"-"`
}

// Fill sets unset fields to field values from otherEvent.
//
// Lists are merged.
func (event *Event) Fill(otherEvent *Event) {
	// Fill unset fields.
	if event.Message == "" {
		event.Message = otherEvent.Message
	}
	if event.EventID == "" {
		event.EventID = otherEvent.EventID
	}
	if event.Project == "" {
		event.Project = otherEvent.Project
	}
	if time.Time(event.Timestamp).IsZero() {
		event.Timestamp = otherEvent.Timestamp
	}
	if event.Level == "" {
		event.Level = otherEvent.Level
	}
	if event.Logger == "" {
		event.Logger = otherEvent.Logger
	}
	if event.Platform == "" {
		event.Platform = otherEvent.Platform
	}
	if event.Culprit == "" {
		event.Culprit = otherEvent.Culprit
	}
	if event.ServerName == "" {
		event.ServerName = otherEvent.ServerName
	}

	// Append
	event.Tags = append(event.Tags, otherEvent.Tags...)
	event.Modules = append(event.Modules, otherEvent.Modules...)
	event.Interfaces = append(event.Interfaces, otherEvent.Interfaces...)

	// Merge
	for k, v := range otherEvent.Extra {
		_, ok := event.Extra[k]
		if !ok {
			if event.Extra == nil {
				event.Extra = make(map[string]interface{}, 1)
			}
			event.Extra[k] = v
		}
	}
}

// FillDefaults sets unset fields to some defaults.
//
// All required fields are set.
func (event *Event) FillDefaults(project string) error {
	defaultEvent := &Event{
		Project:    project,  // Required.
		Level:      ERROR,    // Required.
		Logger:     "root",   // Required.
		Platform:   "go",     // Nice to have.
		ServerName: hostname, // Nice to have.
		Extra: map[string]interface{}{
			"runtime.Version":      runtime.Version(),
			"runtime.NumCPU":       runtime.NumCPU(),
			"runtime.GOMAXPROCS":   runtime.GOMAXPROCS(0), // 0 just returns the current value
			"runtime.NumGoroutine": runtime.NumGoroutine(),
		},
	}
	event.Fill(defaultEvent)

	// Get these required defaults lazily.
	if event.EventID == "" {
		uuid4, err := uuid()
		if err != nil {
			return err
		}
		event.EventID = uuid4
	}
	if time.Time(event.Timestamp).IsZero() {
		event.Timestamp = Timestamp(time.Now())
	}

	// Nice to have, also lazy.
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
