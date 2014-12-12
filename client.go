// Package raven implements a client for the Sentry error logging service.
package raven

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	userAgent = "go-raven/1.0" // Arbitrary (but conventional) string which identifies our client to Sentry.

	MaxQueueBuffer  = 100 // The maximum number of events that will be buffered waiting to be delivered.
	NumContextLines = 5   // Number of pre and post context lines for Capture* methods.
)

type Severity int

var (
	ErrEventDropped        = errors.New("raven: event dropped")
	ErrClientNotConfigured = errors.New("raven: client not configured")
)

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
	DefaultContext *Event

	Transport Transport

	// DropHandler is called when an event is dropped because the buffer is full.
	DropHandler func(*Event)

	mu         sync.RWMutex
	url        string
	projectID  string
	authHeader string
	queue      chan *queuedEvent
}

// NewClient constructs a Sentry client and spawns a background goroutine to
// handle events sent by Client.Report.
func NewClient(dsn string, defaultContext *Event) (*Client, error) {
	client := &Client{Transport: &HTTPTransport{}, DefaultContext: defaultContext, queue: make(chan *queuedEvent, MaxQueueBuffer)}
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

// Capture asynchronously delivers an Event to the Sentry server. It is a no-op
// when client is nil. A channel is provided if it is important to check for a
// send's success.
func (client *Client) Capture(message string, event *Event) (eventID string, ch chan error) {
	ch = make(chan error, 1)

	if client == nil {
		ch <- ErrClientNotConfigured
		return "", ch
	}

	if message == "" {
		ch <- errors.New("raven: no message")
		return "", ch
	}

	// Merge events together.
	mergedEvent := &Event{Message: message}
	mergedEvent.Merge(client.DefaultContext)
	mergedEvent.Merge(event)

	// Fetch the current client config.
	client.mu.RLock()
	project, url, authHeader := client.projectID, client.url, client.authHeader
	client.mu.RUnlock()

	// Fill missing event fields with defaults.
	mergedEvent.FillDefaults(project)

	select {
	case client.queue <- &queuedEvent{event: mergedEvent, url: url, authHeader: authHeader, ch: ch}:
	default:
		// Send would block, drop the event.
		if client.DropHandler != nil {
			client.DropHandler(mergedEvent)
		}
		ch <- ErrEventDropped
	}

	return event.EventID, ch
}

// CaptureMessage formats and delivers a string message to the Sentry server.
func (client *Client) CaptureMessage(message string, captureContext *Event) string {
	event := &Event{Interfaces: []Interface{&Message{message, nil}}}
	event.Merge(captureContext)

	eventID, _ := client.Capture(message, event)

	return eventID
}

// CaptureErrors formats and delivers an errorto the Sentry server.
// Adds a stacktrace to event, excluding the call to this method.
func (client *Client) CaptureError(err error, captureContext *Event) string {
	event := &Event{Interfaces: []Interface{NewException(err, NewStacktrace(1, 3, nil))}}
	event.Merge(captureContext)

	eventID, _ := client.Capture(err.Error(), event)

	return eventID
}

// CapturePanic calls f and then recovers and reports a panic to the Sentry server if it occurs.
func (client *Client) CapturePanic(f func(), captureContext *Event) {
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

		event := &Event{Interfaces: []Interface{NewException(err, NewStacktrace(2, NumContextLines, nil))}}
		client.Capture(err.Error(), event)
	}()

	f()
}

func (client *Client) Close() {
	close(client.queue)
}

// FormatEventID formats and event ID into canonical UUID format for displaying to users.
func FormatEventID(id string) string {
	return id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
}

// queuedEvent represents an event to send on the worker goroutine.
//
// The URL and auth header are stored alongside the event since the event is initialized
// with a project id that must match the URL and auth header.
type queuedEvent struct {
	event      *Event
	url        string
	authHeader string
	ch         chan error
}

func (client *Client) worker() {
	for event := range client.queue {
		event.ch <- client.Transport.Send(event.url, event.authHeader, event.event)
	}
}
