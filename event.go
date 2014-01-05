package raven

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"time"
)

type Tag struct {
	Key   string
	Value string
}

func (tag *Tag) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]string{tag.Key, tag.Value})
}

// http://sentry.readthedocs.org/en/latest/developer/client/index.html#building-the-json-packet
type EventInfo struct {
	// Required
	Message string `json:"message"`

	// Required, set automatically by Client.Send/Report via EventInfo.Init if blank
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

func (eventInfo *EventInfo) Merge(otherEventInfo *EventInfo) *EventInfo {
	if otherEventInfo.Message != "" {
		eventInfo.Message = otherEventInfo.Message
	}
	if otherEventInfo.EventID != "" {
		eventInfo.EventID = otherEventInfo.EventID
	}
	if otherEventInfo.Project != "" {
		eventInfo.Project = otherEventInfo.Project
	}
	if !time.Time(otherEventInfo.Timestamp).IsZero() {
		eventInfo.Timestamp = otherEventInfo.Timestamp
	}
	if otherEventInfo.Level != 0 {
		eventInfo.Level = otherEventInfo.Level
	}
	if otherEventInfo.Logger != "" {
		eventInfo.Logger = otherEventInfo.Logger
	}
	if otherEventInfo.Platform != "" {
		eventInfo.Platform = otherEventInfo.Platform
	}
	if otherEventInfo.Culprit != "" {
		eventInfo.Culprit = otherEventInfo.Culprit
	}
	if otherEventInfo.Tags != nil {
		eventInfo.Tags = append(eventInfo.Tags, otherEventInfo.Tags...)
	}
	if otherEventInfo.ServerName != "" {
		eventInfo.ServerName = otherEventInfo.ServerName
	}
	if otherEventInfo.Modules != nil {
		eventInfo.Modules = append(eventInfo.Modules, otherEventInfo.Modules...)
	}
	for k, v := range otherEventInfo.Extra {
		eventInfo.Extra[k] = v
	}

	return eventInfo
}

func (eventInfo *EventInfo) JSON() []byte {
	eventInfoJSON, _ := json.Marshal(eventInfo)

	interfaces := make(map[string]Interface, len(eventInfo.Interfaces))
	for _, inter := range eventInfo.Interfaces {
		interfaces[inter.Class()] = inter
	}

	if len(interfaces) > 0 {
		interfaceJSON, _ := json.Marshal(interfaces)
		eventInfoJSON[len(eventInfoJSON)-1] = ','
		eventInfoJSON = append(eventInfoJSON, interfaceJSON[1:]...)
	}

	return eventInfoJSON
}

func serializedPacket(eventInfo *EventInfo) (r io.Reader, contentType string) {
	packetJSON := eventInfo.JSON()

	// Only deflate/base64 the eventInfo if it is bigger than 1KB, as there is
	// overhead.
	if len(packetJSON) > 1000 {
		buf := &bytes.Buffer{}
		b64 := base64.NewEncoder(base64.StdEncoding, buf)
		deflate, _ := zlib.NewWriterLevel(b64, zlib.BestCompression)
		deflate.Write(packetJSON)
		deflate.Close()
		b64.Close()
		return buf, "application/octet-stream"
	}
	return bytes.NewReader(packetJSON), "application/json"
}

type event struct {
	*EventInfo
	ch chan error
}
