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

// Severity identifies the severity of an event.
type Severity string

const (
	Debug   Severity = "debug"   // Fine-grained events for normal execution, primarily useful for debugging.
	Info             = "info"    // Coarse-grained events for normal execution.
	Warning          = "warning" // Risk of abnormal execution in the future. For example, running low on disk space.
	Error            = "error"   // Normal execution is not possible, but the application can continue to run.
	Fatal            = "fatal"   // The application is not recoverable and cannot continue to run.
)

// Timestamp is a time.Time that correctly marshals to JSON for Sentry.
type Timestamp time.Time

// timestampFormat is the time.Time layout string used to generate Timestamp
// JSON values for Sentry.
const timestampFormat = `"2006-01-02T15:04:05"`

// MarshalJSON marshals a Timestamp to JSON for Sentry.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(time.Time(t).UTC().Format(timestampFormat)), nil
}

// UnarshalJSON unmarshals a Timestamp from JSON for Sentry.
func (timestamp *Timestamp) UnmarshalJSON(data []byte) error {
	t, err := time.Parse(timestampFormat, string(data))
	if err != nil {
		return err
	}

	*timestamp = Timestamp(t)
	return nil
}

// An Interface is a Sentry data interface for storing structured data. It can be
// identified by its Class, and rendered in Sentry a particular way.
//
// An Interface must implement json.Marshaler or use json struct tags.
//
// Visit http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html
// for more information about Interfaces in Sentry.
type Interface interface {
	// The Sentry class name. Example: sentry.interfaces.Stacktrace
	Class() string
}

// A Culpriter is an Interface that can report which function caused the event.
type Culpriter interface {
	Interface
	Culprit() string
}

// Context is an alias for Event. It helps with the readability of our Capture* methods.
type Context Event

// An Event is the actual event data that gets sent to Sentry.
//
// Visit http://sentry.readthedocs.org/en/latest/developer/client/index.html#building-the-json-packet
// for a discussion of its fields.
type Event struct {
	// Required
	Message string `json:"message"`

	// Required, set automatically by Client.Capture via Event.FillDefaults if blank.
	EventId   string    `json:"event_id"`
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

// Fill sets unset fields to values from contexts.
//
// List-like values are merged where possible. Where a single value must be
// chosen, the event takes priority, and contexts increase in priority from
// left to right.
func (event *Event) Fill(contexts ...*Context) {
	// Contexts to the right take priority, so start with those.
	for i := len(contexts) - 1; i >= 0; i-- {
		// Fill unset fields.
		if event.Message == "" {
			event.Message = contexts[i].Message
		}
		if event.EventId == "" {
			event.EventId = contexts[i].EventId
		}
		if event.Project == "" {
			event.Project = contexts[i].Project
		}
		if time.Time(event.Timestamp).IsZero() {
			event.Timestamp = contexts[i].Timestamp
		}
		if event.Level == "" {
			event.Level = contexts[i].Level
		}
		if event.Logger == "" {
			event.Logger = contexts[i].Logger
		}
		if event.Platform == "" {
			event.Platform = contexts[i].Platform
		}
		if event.Culprit == "" {
			event.Culprit = contexts[i].Culprit
		}
		if event.ServerName == "" {
			event.ServerName = contexts[i].ServerName
		}

		// Append
		event.Tags = append(event.Tags, contexts[i].Tags...)
		event.Modules = append(event.Modules, contexts[i].Modules...)
		event.Interfaces = append(event.Interfaces, contexts[i].Interfaces...)

		// Merge
		for k, v := range contexts[i].Extra {
			_, ok := event.Extra[k]
			if !ok {
				if event.Extra == nil {
					event.Extra = make(map[string]interface{}, 1)
				}
				event.Extra[k] = v
			}
		}
	}
}

// FillDefaults sets unset fields to some defaults.
//
// All required fields are set.
func (event *Event) FillDefaults(project string) error {
	defaultContext := &Context{
		Project:    project,  // Required.
		Level:      Error,    // Required.
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
	event.Fill(defaultContext)

	// Get these required defaults lazily.
	if event.EventId == "" {
		uuid4, err := uuid()
		if err != nil {
			return err
		}
		event.EventId = uuid4
	}
	if time.Time(event.Timestamp).IsZero() {
		event.Timestamp = Timestamp(time.Now())
	}

	// If culprit is unset, assign a Culprit if possible.
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

var hostname string

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
