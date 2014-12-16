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
	userAgent       = "go-raven/1.0" // Arbitrary (but conventional) string which identifies our client to Sentry.
	timestampFormat = `"2006-01-02T15:04:05"`
	MaxQueueBuffer  = 100 // The maximum number of events that will be buffered waiting to be delivered.
	NumContextLines = 5   // Number of pre and post context lines for Capture* methods.
)

type Severity string

var (
	ErrEventDropped        = errors.New("raven: event dropped")
	ErrClientNotConfigured = errors.New("raven: client not configured")
)

// http://docs.python.org/2/howto/logging.html#logging-levels
const (
	DEBUG   Severity = "debug"
	INFO             = "info"
	WARNING          = "warning"
	ERROR            = "error"
	FATAL            = "fatal"
)

type Timestamp time.Time

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(time.Time(t).UTC().Format(timestampFormat)), nil
}

func (timestamp *Timestamp) UnmarshalJSON(data []byte) error {
	t, err := time.Parse(timestampFormat, string(data))
	if err != nil {
		return err
	}

	*timestamp = Timestamp(t)
	return nil
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
	DefaultContext *Context

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
func NewClient(dsn string, defaultContext *Context) (*Client, error) {
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
func (client *Client) Capture(event *Event) (eventID string, ch chan error) {
	ch = make(chan error, 1)

	if client == nil {
		ch <- ErrClientNotConfigured
		return "", ch
	}

	if event.Message == "" {
		ch <- errors.New("raven: no message")
		return "", ch
	}

	// Fill event with default context.
	event.Fill(client.DefaultContext)

	// Fetch the current client config.
	client.mu.RLock()
	project, url, authHeader := client.projectID, client.url, client.authHeader
	client.mu.RUnlock()

	// Fill missing event fields with defaults.
	event.FillDefaults(project)

	select {
	case client.queue <- &queuedEvent{event: event, url: url, authHeader: authHeader, ch: ch}:
	default:
		// Send would block, drop the event.
		if client.DropHandler != nil {
			client.DropHandler(event)
		}
		ch <- ErrEventDropped
	}

	return event.EventID, ch
}

// CaptureMessage formats and delivers a string message to the Sentry server.
//
// Contexts to the right have higher priority than contexts on the left.
func (client *Client) CaptureMessage(message string, contexts ...*Context) (string, chan error) {
	event := &Event{Message: message}
	event.Fill(contexts...)
	eventID, ch := client.Capture(event)

	return eventID, ch
}

// CaptureErrors formats and delivers an error to the Sentry server.
//
// Adds a stacktrace to event, excluding the call to this method. Contexts
// to the right have higher priority than contexts on the left.
func (client *Client) CaptureError(err error, contexts ...*Context) (string, chan error) {
	event := &Event{Interfaces: []Interface{NewException(err, NewStacktrace(1, 3, nil))}}
	event.Fill(contexts...)

	// If capture context didn't have a message, set one.
	if event.Message == "" {
		event.Message = err.Error()
	}

	eventID, ch := client.Capture(event)

	return eventID, ch
}

// CapturePanic calls f and then recovers and reports a panic to the Sentry server if it occurs.
//
// Contexts to the right have higher priority than contexts on the left.
func (client *Client) CapturePanic(f func(), contexts ...*Context) {
	if client == nil {
		return
	}

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

		event := &Event{Message: err.Error(), Interfaces: []Interface{NewException(err, NewStacktrace(2, NumContextLines, nil))}}
		event.Fill(contexts...)
		client.Capture(event)
	}()

	f()
}

func (client *Client) Close() {
	close(client.queue)
}

func (client *Client) ProjectID() string {
	client.mu.RLock()
	defer client.mu.RUnlock()

	return client.projectID
}

func (client *Client) URL() string {
	client.mu.RLock()
	defer client.mu.RUnlock()

	return client.url
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
