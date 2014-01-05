// Package raven implements a client for the Sentry error logging service.
package raven

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	userAgent = "go-raven/1.0" // Arbitrary (but conventional) string which identifies our client to Sentry.

	MaxQueueBuffer  = 100 // The maximum number of packets that will be buffered waiting to be delivered.
	NumContextLines = 5   // Number of pre and post context lines for Capture* methods.
)

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

// Client encapsulates a connection to a Sentry server. It must be initialized
// by calling NewClient. Modification of fields concurrently with Send or after
// calling Report for the first time is not thread-safe.
type Client struct {
	DefaultEventInfo *EventInfo

	Transport Transport

	// DropHandler is called when an event is dropped because the buffer is full.
	DropHandler func(*EventInfo)

	mu         sync.RWMutex
	url        string
	projectID  string
	authHeader string
	queue      chan *event
}

// NewClient constructs a Sentry client and spawns a background goroutine to
// handle packets sent by Client.Report.
func NewClient(dsn string, defaultEventInfo *EventInfo) (*Client, error) {
	client := &Client{Transport: &HTTPTransport{}, DefaultEventInfo: defaultEventInfo, queue: make(chan *event, MaxQueueBuffer)}
	go client.worker()
	return client, client.SetDSN(dsn)
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

// Capture asynchronously delivers a eventInfo to the Sentry server. It is a no-op
// when client is nil. A channel is provided if it is important to check for a
// send's success.
func (client *Client) Capture(message string, eventInfo *EventInfo) (eventID string, ch chan error) {
	ch = make(chan error, 1)

	if client == nil {
		ch <- errors.New("raven: client not configured")
		return
	}

	event, err := client.newEvent(message, eventInfo, ch)
	if err != nil {
		ch <- err
		return
	}

	select {
	case client.queue <- event:
	default:
		// Send would block, drop the event
		if client.DropHandler != nil {
			client.DropHandler(event.EventInfo)
		}
		ch <- ErrEventDropped
	}

	return event.EventID, ch
}

// CaptureMessage formats and delivers a string message to the Sentry server.
func (client *Client) CaptureMessage(message string, captureContext *EventInfo) string {
	eventInfo := &EventInfo{Interfaces: []Interface{&Message{message, nil}}}
	eventInfo.Merge(captureContext)

	eventID, _ := client.Capture(message, eventInfo)

	return eventID
}

// CaptureErrors formats and delivers an errorto the Sentry server.
// Adds a stacktrace to eventInfo, excluding the call to this method.
func (client *Client) CaptureError(err error, captureContext *EventInfo) string {
	eventInfo := &EventInfo{Interfaces: []Interface{NewException(err, NewStacktrace(1, 3, nil))}}
	eventInfo.Merge(captureContext)

	eventID, _ := client.Capture(err.Error(), eventInfo)

	return eventID
}

// CapturePanic calls f and then recovers and reports a panic to the Sentry server if it occurs.
func (client *Client) CapturePanic(f func(), captureContext *EventInfo) {
	defer func() {
		rval := recover()
		if rval == nil {
			return
		}

		var err error
		switch rval := rval.(type) {
		case error:
			// If this is an error type, pass it on directly. This preserves error type.
			err = rval
		default:
			// If recover is (non-conventionally) not an error, make it one.
			err = fmt.Errorf("%v", rval)
		}

		eventInfo := &EventInfo{Interfaces: []Interface{NewException(err, NewStacktrace(2, NumContextLines, nil))}}
		client.Capture(err.Error(), eventInfo)
	}()

	f()
}

func (client *Client) Close() {
	close(client.queue)
}

func (client *Client) newEvent(message string, eventInfo *EventInfo, ch chan error) (*event, error) {
	if message == "" {
		return nil, errors.New("raven: no message")
	}

	uuid4, err := uuid()
	if err != nil {
		return nil, err
	}

	client.mu.RLock()
	projectID := client.projectID
	client.mu.RUnlock()
	if projectID == "" {
		return nil, errors.New("raven: client not configured")
	}

	// Merge interfaces together
	mergedInfo := &EventInfo{Message: message, EventID: uuid4, Project: projectID}
	mergedInfo.Merge(client.DefaultEventInfo)
	mergedInfo.Merge(eventInfo)

	if time.Time(mergedInfo.Timestamp).IsZero() {
		mergedInfo.Timestamp = Timestamp(time.Now())
	}
	if mergedInfo.Level == 0 {
		mergedInfo.Level = ERROR
	}
	if mergedInfo.Logger == "" {
		mergedInfo.Logger = "root"
	}
	if mergedInfo.ServerName == "" {
		mergedInfo.ServerName = hostname
	}
	if mergedInfo.Culprit == "" {
		for _, inter := range mergedInfo.Interfaces {
			if c, ok := inter.(Culpriter); ok {
				mergedInfo.Culprit = c.Culprit()
				if mergedInfo.Culprit != "" {
					break
				}
			}
		}
	}

	return &event{mergedInfo, ch}, nil
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

// FormatEventID formats and event ID into canonical UUID format for displaying to users.
func FormatEventID(id string) string {
	return id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
}

func (client *Client) worker() {
	for event := range client.queue {
		client.mu.RLock()
		url, authHeader := client.url, client.authHeader
		client.mu.RUnlock()

		event.ch <- client.Transport.Send(url, authHeader, event.EventInfo)
	}
}

var hostname string

func init() {
	hostname, _ = os.Hostname()
}
