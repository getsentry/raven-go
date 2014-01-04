// Package raven implements a client for the Sentry error logging service.
package raven

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const userAgent = "go-raven/1.0"

type Severity int

var ErrEventDropped = errors.New("raven: event dropped")

// http://docs.python.org/2/howto/logging.html#logging-levels
const (
	DEBUG Severity = (iota + 1) * 10
	INFO
	WARNING
	ERROR
	FATAL
)

type Timestamp time.Time

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(time.Time(t).UTC().Format(`"2006-01-02T15:04:05"`)), nil
}

// An Interface is a Sentry interface that will be serialized as JSON.
// It must implement json.Marshaler or use json struct tags.
type Interface interface {
	// The Sentry class name. Example: sentry.interfaces.Stacktrace
	Class() string
}

type Culpriter interface {
	Culprit() string
}

type Transport interface {
	Send(url, authHeader string, eventInfo *EventInfo) error
}

type event struct {
	info *EventInfo
	ch   chan error
}

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

// NewPacket constructs a packet with the specified message and interfaces.
func NewEventInfo(message string, interfaces ...Interface) *EventInfo {
	return &EventInfo{Message: message, Interfaces: interfaces, Extra: make(map[string]interface{})}
}

// Init initializes required fields in event info. It is typically called by
// Client.Send/Report automatically.
func (eventInfo *EventInfo) Init(project string) error {
	if eventInfo.Message == "" {
		return errors.New("raven: empty message")
	}
	if eventInfo.Project == "" {
		eventInfo.Project = project
	}
	if eventInfo.EventID == "" {
		var err error
		eventInfo.EventID, err = uuid()
		if err != nil {
			return err
		}
	}
	if time.Time(eventInfo.Timestamp).IsZero() {
		eventInfo.Timestamp = Timestamp(time.Now())
	}
	if eventInfo.Level == 0 {
		eventInfo.Level = ERROR
	}
	if eventInfo.Logger == "" {
		eventInfo.Logger = "root"
	}
	if eventInfo.ServerName == "" {
		eventInfo.ServerName = hostname
	}

	if eventInfo.Culprit == "" {
		for _, inter := range eventInfo.Interfaces {
			if c, ok := inter.(Culpriter); ok {
				eventInfo.Culprit = c.Culprit()
				if eventInfo.Culprit != "" {
					break
				}
			}
		}
	}

	return nil
}

func (eventInfo *EventInfo) AddTags(tags map[string]string) {
	for k, v := range tags {
		eventInfo.Tags = append(eventInfo.Tags, Tag{k, v})
	}
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

func FormatUUID(id string) string {
	return id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
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

// The maximum number of packets that will be buffered waiting to be delivered.
// Packets will be dropped if the buffer is full. Used by NewClient.
var MaxQueueBuffer = 100

// NewClient constructs a Sentry client and spawns a background goroutine to
// handle packets sent by Client.Report.
func NewClient(dsn string, tags map[string]string) (*Client, error) {
	client := &Client{Transport: &HTTPTransport{}, Tags: tags, queue: make(chan *event, MaxQueueBuffer)}
	go client.worker()
	return client, client.SetDSN(dsn)
}

// Client encapsulates a connection to a Sentry server. It must be initialized
// by calling NewClient. Modification of fields concurrently with Send or after
// calling Report for the first time is not thread-safe.
type Client struct {
	Tags map[string]string

	Transport Transport

	// DropHandler is called when an event is dropped because the buffer is full.
	DropHandler func(*EventInfo)

	mu         sync.RWMutex
	url        string
	projectID  string
	authHeader string
	queue      chan *event
}

// SetDSN updates a client with a new DSN. It safe to call after and
// concurrently with calls to Report and Send.
func (client *Client) SetDSN(dsn string) error {
	if dsn == "" {
		return nil
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	uri, err := url.Parse(dsn)
	if err != nil {
		return err
	}

	if uri.User == nil {
		return errors.New("raven: dsn missing public key and/or private key")
	}
	publicKey := uri.User.Username()
	secretKey, ok := uri.User.Password()
	if !ok {
		return errors.New("raven: dsn missing private key")
	}
	uri.User = nil

	if idx := strings.LastIndex(uri.Path, "/"); idx != -1 {
		client.projectID = uri.Path[idx+1:]
		uri.Path = uri.Path[:idx+1] + "api/" + client.projectID + "/store/"
	}
	if client.projectID == "" {
		return errors.New("raven: dsn missing project id")
	}

	client.url = uri.String()

	client.authHeader = fmt.Sprintf("Sentry sentry_version=4, sentry_key=%s, sentry_secret=%s", publicKey, secretKey)

	return nil
}

func (client *Client) worker() {
	for event := range client.queue {
		client.mu.RLock()
		url, authHeader := client.url, client.authHeader
		client.mu.RUnlock()

		event.ch <- client.Transport.Send(url, authHeader, event.info)
	}
}

// Capture asynchronously delivers a eventInfo to the Sentry server. It is a no-op
// when client is nil. A channel is provided if it is important to check for a
// send's success.
func (client *Client) Capture(eventInfo *EventInfo, captureTags map[string]string) (eventID string, ch chan error) {
	if client == nil {
		return
	}

	ch = make(chan error, 1)

	// Merge capture tags and client tags
	eventInfo.AddTags(captureTags)
	eventInfo.AddTags(client.Tags)

	// Initialize any required eventInfo fields
	client.mu.RLock()
	projectID := client.projectID
	client.mu.RUnlock()

	err := eventInfo.Init(projectID)
	if err != nil {
		ch <- err
		return
	}

	event := &event{eventInfo, ch}

	select {
	case client.queue <- event:
	default:
		// Send would block, drop the eventInfo
		if client.DropHandler != nil {
			client.DropHandler(eventInfo)
		}
		ch <- ErrEventDropped
	}

	return eventInfo.EventID, ch
}

// CaptureMessage formats and delivers a string message to the Sentry server.
func (client *Client) CaptureMessage(message string, tags map[string]string, interfaces ...Interface) string {
	eventInfo := NewEventInfo(message, append(interfaces, &Message{message, nil})...)
	eventID, _ := client.Capture(eventInfo, tags)

	return eventID
}

// CaptureErrors formats and delivers an errorto the Sentry server.
// Adds a stacktrace to eventInfo, excluding the call to this method.
func (client *Client) CaptureError(err error, tags map[string]string, interfaces ...Interface) string {
	eventInfo := NewEventInfo(err.Error(), append(interfaces, NewException(err, NewStacktrace(1, 3, nil)))...)
	eventID, _ := client.Capture(eventInfo, tags)

	return eventID
}

// CapturePanic calls f and then recovers and reports a panic to the Sentry server if it occurs.
func (client *Client) CapturePanic(f func(), tags map[string]string, interfaces ...Interface) {
	defer func() {
		var eventInfo *EventInfo
		switch rval := recover().(type) {
		case nil:
			return
		case error:
			eventInfo = NewEventInfo(rval.Error(), append(interfaces, NewException(rval, NewStacktrace(2, 3, nil)))...)
		default:
			rvalStr := fmt.Sprint(rval)
			eventInfo = NewEventInfo(rvalStr, append(interfaces, NewException(errors.New(rvalStr), NewStacktrace(2, 3, nil)))...)
		}

		client.Capture(eventInfo, tags)
	}()

	f()
}

func (client *Client) Close() {
	close(client.queue)
}

// HTTPTransport is the default transport, delivering packets to Sentry via the
// HTTP API.
type HTTPTransport struct {
	http http.Client
}

func (t *HTTPTransport) Send(url, authHeader string, eventInfo *EventInfo) error {
	if url == "" {
		return nil
	}

	body, contentType := serializedPacket(eventInfo)
	req, _ := http.NewRequest("POST", url, body)
	req.Header.Set("X-Sentry-Auth", authHeader)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", contentType)
	res, err := t.http.Do(req)
	if err != nil {
		return err
	}
	io.Copy(ioutil.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("raven: got http status %d", res.StatusCode)
	}
	return nil
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

var hostname string

func init() {
	hostname, _ = os.Hostname()
}
