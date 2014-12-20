package raven

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

const (
	EventQueueSize  = 100 // The maximum number of events that will be buffered waiting to be delivered.
	NumContextLines = 5   // Number of pre- and post- context lines for Capture* methods.
)

// Client encapsulates a connection to a Sentry server. A Client must be created with NewClient.
type Client struct {
	defaultContext *Context
	transport      Transport
	dropHandler    func(*Event)

	mu         sync.RWMutex
	url        string
	projectId  string
	authHeader string

	queue chan *queuedEvent
}

// NewClient constructs a Sentry Client.
//
// NewClient also spawns a worker goroutine to handle events sent by Client.Capture.
// If no transport is set in the ClientConfig, the default HTTP transport is used.
func NewClient(dsn string, config ClientConfig) (*Client, error) {
	client := &Client{
		defaultContext: config.DefaultContext,
		dropHandler:    config.DropHandler,
		queue:          make(chan *queuedEvent, EventQueueSize),
	}

	if config.Transport != nil {
		client.transport = config.Transport
	} else {
		client.transport = &HTTPTransport{}
	}

	go client.worker()

	return client, client.SetDSN(dsn)
}

// SetDSN updates a client with a new DSN. It safe to call before, after, and
// concurrently with calls to Capture and Send.
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
		client.projectId = uri.Path[idx+1:]
		uri.Path = uri.Path[:idx+1] + "api/" + client.projectId + "/store/"
	}
	if client.projectId == "" {
		return errors.New("raven: dsn missing project id")
	}

	client.url = uri.String()

	client.authHeader = fmt.Sprintf("Sentry sentry_version=4, sentry_key=%s, sentry_secret=%s", publicKey, secretKey)

	return nil
}

// Capture asynchronously delivers an event to the Sentry server.
//
// It is a no-op when client is nil. An error channel is provided if it is
// important to receive a response from the Sentry server.
func (client *Client) Capture(event *Event) (eventId string, ch chan error) {
	ch = make(chan error, 1)

	if client == nil {
		ch <- errors.New("raven: client not configured")
		return "", ch
	}

	if event.Message == "" {
		ch <- errors.New("raven: no message")
		return "", ch
	}

	// Fill missing event fields with default context.
	event.Fill(client.defaultContext)

	select {
	case client.queue <- &queuedEvent{event: event, ch: ch}:
	default:
		// Send would block, drop the event.
		if client.dropHandler != nil {
			client.dropHandler(event)
		}
		ch <- errors.New("raven: event dropped")
	}

	return event.EventId, ch
}

// CaptureMessage formats and asynchronously delivers a message event to the Sentry server.
//
// It is a no-op when client is nil. Contexts increase in priority from left to right.
// An error channel is provided if it is important to receive a response from the Sentry
// server.
func (client *Client) CaptureMessage(message string, contexts ...*Context) (eventId string, ch chan error) {
	event := &Event{Message: message}
	event.Fill(contexts...)

	return client.Capture(event)
}

// CaptureError formats and asynchronously delivers an error event to the Sentry server.
//
// It is a no-op when client is nil. Contexts increase in priority from left to right.
// An error channel is provided if it is important to receive a response from the Sentry
// server.
func (client *Client) CaptureError(err error, contexts ...*Context) (string, chan error) {
	event := &Event{Interfaces: []Interface{NewException(err, NewStacktrace(1, NumContextLines, nil))}}
	event.Fill(contexts...)

	// If capture context didn't have a message, set one.
	if event.Message == "" {
		event.Message = err.Error()
	}

	return client.Capture(event)
}

// CapturePanic calls f and will recover and reports a panics to the Sentry server.
//
// If client is nil, f will still be called and panics will still be recovered from.
// Contexts increase in priority from left to right.
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

// Close cleans up a client after it is no longer needed.
//
// The worker goroutine will stop.
func (client *Client) Close() {
	close(client.queue)
}

// ProjectId is a thread-safe way to get the project id of the client.
func (client *Client) ProjectId() string {
	client.mu.RLock()
	defer client.mu.RUnlock()

	return client.projectId
}

// URL is a thread-safe way to get the URL of the client.
func (client *Client) URL() string {
	client.mu.RLock()
	defer client.mu.RUnlock()

	return client.url
}

// ClientConfig defines a set of optional configuration variables for a Client.
type ClientConfig struct {
	// DefaultContext is used to fill unset fields on an event before sending the event.
	DefaultContext *Context

	// DropHandler is called when an event is dropped because the buffer is full.
	DropHandler func(*Event)

	// Transport is a specific transport to use for event delivery.
	Transport Transport
}

// queuedEvent represents an event to send on the worker goroutine.
//
// It includes a return channel for reporting errors.
type queuedEvent struct {
	event *Event
	ch    chan error
}

// worker receives queued events from the event queue and uses the transport to deliver them.
//
// Any unset required event fields are set.
func (client *Client) worker() {
	for e := range client.queue {
		// Fetch the current client config.
		client.mu.RLock()
		projectId, url, authHeader := client.projectId, client.url, client.authHeader
		client.mu.RUnlock()

		// Fill unset required event fields with defaults.
		e.event.FillDefaults(projectId)

		e.ch <- client.transport.Send(url, authHeader, e.event)
	}
}
