package raven

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	EventQueueSize  = 100 // The maximum number of events that will be buffered waiting to be delivered.
	NumContextLines = 5   // Number of pre- and post- context lines for Capture* methods.
)

// A Client is used to send events to a Sentry server. All Clients must be created
// with NewClient.
type Client struct {
	context     *Context
	transport   Transport
	dropHandler func(*Event)

	mu         sync.RWMutex
	url        string
	projectId  string
	authHeader string

	queue chan *queuedEvent
}

// NewClient creates a Sentry Client. It is the caller's resposibility to call Close on
// the Client when finished.
//
// All ClientConfig settings are optional. NewClient spawns a worker goroutine to handle
// events sent by Client.Capture.
func NewClient(dsn string, config ClientConfig) (*Client, error) {
	var transport Transport
	if config.Transport != nil {
		transport = config.Transport
	} else {
		transport = &HTTPTransport{}
	}

	client := &Client{
		context: &Context{
			Level:      config.DefaultLevel,
			Tags:       config.Tags,
			ServerName: config.DefaultServerName,
			Modules:    config.Modules,
			Extra:      config.Extra,
			Interfaces: config.Interfaces,
		},
		transport:   transport,
		dropHandler: config.DropHandler,
		queue:       make(chan *queuedEvent, EventQueueSize),
	}

	go client.worker()

	return client, client.SetDSN(dsn)
}

// SetDSN updates a client with a new DSN. It safe to call before, after, and
// concurrently with calls to the Capture methods.
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

// CaptureMessage formats and asynchronously delivers a message event to the Sentry server.
//
// It is a no-op when client is nil. Contexts increase in priority from left to right.
// An error channel is provided if it is important to receive a response from the Sentry
// server.
func (client *Client) CaptureMessage(message string, contexts ...*Context) (eventId string, ch chan error) {
	event := &Event{Message: message}
	event.fill(contexts...)

	return client.capture(event)
}

// CaptureError formats and asynchronously delivers an error event to the Sentry server.
// CaptureError includes a stacktrace.
//
// It is a no-op when client is nil. Contexts increase in priority from left to right.
// An error channel is provided if it is important to receive a response from the Sentry
// server.
func (client *Client) CaptureError(err error, contexts ...*Context) (string, chan error) {
	event := &Event{Interfaces: []Interface{NewException(err, NewStacktrace(1, NumContextLines, nil))}}
	event.fill(contexts...)

	// If capture context didn't have a message, set one.
	if event.Message == "" {
		event.Message = err.Error()
	}

	return client.capture(event)
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
		event.fill(contexts...)
		client.capture(event)
	}()

	f()
}

// URL is a thread-safe way to get the URL of the client.
func (client *Client) URL() string {
	client.mu.RLock()
	defer client.mu.RUnlock()

	return client.url
}

// ProjectId is a thread-safe way to get the project id of the client.
func (client *Client) ProjectId() string {
	client.mu.RLock()
	defer client.mu.RUnlock()

	return client.projectId
}

// Close cleans up a client after it is no longer needed.
//
// The worker goroutine will stop.
func (client *Client) Close() {
	close(client.queue)
}

// capture asynchronously delivers an event to the Sentry server.
//
// It is a no-op when client is nil. An error channel is provided if it is
// important to receive a response from the Sentry server.
func (client *Client) capture(event *Event) (eventId string, ch chan error) {
	ch = make(chan error, 1)

	if client == nil {
		ch <- errors.New("raven: client not configured")
		return "", ch
	}

	if event.Message == "" {
		ch <- errors.New("raven: no message")
		return "", ch
	}

	// Fill the event with as many sensible defaults as possible, and get a queuedEvent.
	queuedEvent, err := client.finalizeEvent(event, ch)
	if err != nil {
		ch <- err
		return "", ch
	}

	select {
	case client.queue <- queuedEvent:
	default:
		// Send would block, drop the event.
		if client.dropHandler != nil {
			client.dropHandler(event)
		}
		ch <- errors.New("raven: event dropped")
	}

	return event.EventId, ch
}

// finalizeEvent processes the event to fill as many sensible defaults as possible,
// and prepares a queuedEvent with a hostname and authHeader matching event.Project.
func (client *Client) finalizeEvent(event *Event, ch chan error) (*queuedEvent, error) {
	// Fill missing event fields with client context.
	event.fill(client.context)

	// Fill missing event fields with a sensible, timely default context.
	client.mu.RLock()
	project, url, authHeader := client.projectId, client.url, client.authHeader
	client.mu.RUnlock()

	eventId, err := uuid()
	if err != nil {
		return nil, err
	}

	event.fill(&Context{
		EventId:    eventId,
		Project:    project,
		Timestamp:  Timestamp(time.Now()),
		Level:      Error,
		Logger:     "root",
		Platform:   "go",
		ServerName: hostname, // Global
		Extra: map[string]interface{}{
			"runtime.Version":      runtime.Version(),
			"runtime.NumCPU":       runtime.NumCPU(),
			"runtime.GOMAXPROCS":   runtime.GOMAXPROCS(0), // 0 just returns the current value
			"runtime.NumGoroutine": runtime.NumGoroutine(),
		},
	})

	// Attempt to derive a Culprit if Culprit is unset.
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

	return &queuedEvent{event: event, url: url, authHeader: authHeader, ch: ch}, nil
}

// worker receives queued events from the event queue and uses the transport to deliver them.
//
// Any unset required event fields are set.
func (client *Client) worker() {
	for e := range client.queue {
		e.ch <- client.transport.Send(e.url, e.authHeader, e.event)
	}
}

// uuid generates a UUIDv4 for a unique event id.
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

// ClientConfig defines a set of optional configuration variables for a Client.
type ClientConfig struct {
	// These default fields are merged with all outgoing events.
	DefaultLevel      Severity
	Tags              Tags
	DefaultServerName string
	Modules           []map[string]string
	Extra             map[string]interface{}
	Interfaces        []Interface

	// DropHandler is called when an event is dropped because the buffer is full.
	DropHandler func(*Event)

	// Transport is a specific transport to use for event delivery.
	Transport Transport
}

// queuedEvent represents an event to send on the worker goroutine.
//
// url and authHeader are sent alongside the event to ensure they are matched with
// the event's project. It includes a return channel for reporting errors.
type queuedEvent struct {
	event      *Event
	url        string
	authHeader string
	ch         chan error
}

var hostname string

// init sets hostname so it doesn't need to be called for every Capture.
func init() {
	hostname, _ = os.Hostname()
}
