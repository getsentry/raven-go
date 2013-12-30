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

// ErrPacketDropped is returned when buffering a packet would block.
// Client.DropHandler is called for dropped packet.
var ErrPacketDropped = errors.New("raven: event dropped")

// An arbitrary string which identifies our client to Sentry, including
// our client version.
// http://sentry.readthedocs.org/en/latest/developer/client/index.html#sentry_client
const userAgent = "go-raven/1.0"

// http://docs.python.org/2/howto/logging.html#logging-levels
const (
	DEBUG Severity = (iota + 1) * 10
	INFO
	WARNING
	ERROR
	FATAL
)

// An Interface is a Sentry interface that will be serialized as JSON.
// It must implement json.Marshaler or use json struct tags.
type Interface interface {
	// The Sentry class name. Example: sentry.interfaces.Stacktrace
	Class() string
}

// A Tag is a key/value pair that describes a Sentry event.
// http://sentry.readthedocs.org/en/latest/developer/client/index.html#id1
type Tag struct {
	Key   string
	Value string
}

// MarshalJSON converts a Tag to a JSON for Sentry. Sentry expects tags to
// be represented by an array with two values, the key followed by the value.
func (tag *Tag) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]string{tag.Key, tag.Value})
}

// These types allow simple values to be passed as variadic arguments and
// mapped to fields in the packet struct.
type Timestamp time.Time
type Severity int
type Logger string
type Platform string
type Culprit string
type ServerName string
type Module map[string]string
type Extra map[string]interface{}

// MarshalJSON converts a time.Time struct into the JSON format expected by Sentry.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(time.Time(t).UTC().Format(`"2006-01-02T15:04:05"`)), nil
}

// A packet represents the required and optional information that can be captured as a
// Sentry event.
// http://sentry.readthedocs.org/en/latest/developer/client/index.html#building-the-json-packet
type packet struct {
	// Set automatically by Client.NewPacket
	Project string `json:"project"`
	EventID string `json:"event_id"`

	// Required
	Message string `json:"message"`

	// Required, set automatically by Client.NewPacket if blank
	Timestamp Timestamp `json:"timestamp"`
	Level     Severity  `json:"level"`
	Logger    Logger    `json:"logger"`

	// Optional
	Platform   Platform   `json:"platform,omitempty"`
	Culprit    Culprit    `json:"culprit,omitempty"`
	Tags       []Tag      `json:"tags,omitempty"`
	ServerName ServerName `json:"server_name,omitempty"`
	Modules    []Module   `json:"modules,omitempty"`
	Extra      Extra      `json:"extra,omitempty"`

	// Optional interfaces
	Interfaces []Interface `json:"-"`

	// Return channel
	ch chan error
}

// A Culpriter is an attribute that can derive the culprit of a Sentry event.
type Culpriter interface {
	Culprit() Culprit
}

// A Transport implements a Send method that delivers packets to Sentry.
type Transport interface {
	Send(url, authHeader string, packet *packet) error
}

// uuid constructs a UUID4 string.
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

// FormatUUID converts a UUID4 string to standard hyphenated format.
func FormatUUID(id string) string {
	return id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
}

// MergeAttributes merges packet attributes together. New attributes replace
// or are concatenated with existing attributes.
func (packet *packet) MergeAttributes(attr []interface{}) error {
	for _, a := range attr {
		switch attrVal := a.(type) {
		case Timestamp:
			packet.Timestamp = attrVal
		case Severity:
			packet.Level = attrVal
		case Logger:
			packet.Logger = attrVal
		case Platform:
			packet.Platform = attrVal
		case Culprit:
			packet.Culprit = attrVal
		case []Tag:
			for _, tag := range attrVal {
				packet.Tags = append(packet.Tags, tag)
			}
		case Tag:
			packet.Tags = append(packet.Tags, attrVal)
		case ServerName:
			packet.ServerName = attrVal
		case []Module:
			for _, module := range attrVal {
				packet.Modules = append(packet.Modules, module)
			}
		case Module:
			packet.Modules = append(packet.Modules, attrVal)
		case Extra:
			for tag, val := range attrVal {
				packet.Extra[tag] = val
			}
		case Interface:
			packet.Interfaces = append(packet.Interfaces, attrVal)
		default:
			return errors.New("raven: bad interface")
		}
	}

	return nil
}

func (packet *packet) JSON() []byte {
	packetJSON, _ := json.Marshal(packet)

	interfaces := make(map[string]Interface, len(packet.Interfaces))
	for _, inter := range packet.Interfaces {
		interfaces[inter.Class()] = inter
	}

	if len(interfaces) > 0 {
		interfaceJSON, _ := json.Marshal(interfaces)
		packetJSON[len(packetJSON)-1] = ','
		packetJSON = append(packetJSON, interfaceJSON[1:]...)
	}

	return packetJSON
}

// The maximum number of packets that will be buffered waiting to be delivered.
// Packets will be dropped if the buffer is full. Used by NewClient.
var MaxQueueBuffer = 100

// NewClient constructs a Sentry client and spawns a background goroutine to
// handle packets sent by Client.Report.
func NewClient(dsn string, attr ...interface{}) (*Client, error) {
	client := &Client{Transport: &HTTPTransport{}, Attributes: attr, queue: make(chan *packet, MaxQueueBuffer)}
	go client.worker()
	return client, client.SetDSN(dsn)
}

// Client encapsulates a connection to a Sentry server. It must be initialized
// by calling NewClient. Modification of fields concurrently with Send or after
// calling Report for the first time is not thread-safe.
type Client struct {
	Attributes []interface{}

	Transport Transport

	// DropHandler is called when a packet is dropped because the buffer is full.
	DropHandler func(*packet)

	mu         sync.RWMutex
	url        string
	projectID  string
	authHeader string
	queue      chan *packet
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
	for packet := range client.queue {
		client.mu.RLock()
		url, authHeader := client.url, client.authHeader
		client.mu.RUnlock()

		packet.ch <- client.Transport.Send(url, authHeader, packet)
	}
}

// Capture asynchronously delivers a packet to the Sentry server. It is a no-op
// when client is nil. A channel is provided if it is important to check for a
// send's success.
func (client *Client) Capture(message string, attr ...interface{}) (eventID string, ch chan error) {
	if client == nil {
		return
	}

	packet, err := client.NewPacket(message, attr...)
	if err != nil {
		packet.ch <- err
		return
	}

	select {
	case client.queue <- packet:
	default:
		// Send would block, drop the packet
		if client.DropHandler != nil {
			client.DropHandler(packet)
		}
		packet.ch <- ErrPacketDropped
	}

	return packet.EventID, packet.ch
}

// CaptureMessage formats and delivers a string message to the Sentry server.
func (client *Client) CaptureMessage(message string, attr ...interface{}) string {
	eventID, _ := client.Capture(message, append(attr, &Message{message, nil})...)
	return eventID
}

// CaptureErrors formats and delivers an errorto the Sentry server.
// Adds a stacktrace to the packet, excluding the call to this method.
func (client *Client) CaptureError(err error, attr ...interface{}) string {
	eventID, _ := client.Capture(err.Error(), append(attr, NewException(err, NewStacktrace(1, 3, nil)))...)
	return eventID
}

// CapturePanic calls f and then recovers and reports a panic to the Sentry server if it occurs.
func (client *Client) CapturePanic(f func(), attr ...interface{}) {
	defer func() {
		switch rval := recover().(type) {
		case nil:
			return
		case error:
			client.Capture(rval.Error(), append(attr, NewException(rval, NewStacktrace(2, 3, nil)))...)
		default:
			rvalStr := fmt.Sprint(rval)
			client.Capture(rvalStr, append(attr, NewException(errors.New(rvalStr), NewStacktrace(2, 3, nil)))...)
		}
	}()

	f()
}

// NewPacket constructs a packet for delivery to Sentry. Client attributes are merged with
// with the specified message and attributes, any unset required fields are initialized
// to their default values, and a culprit is derived if one hasn't been specified.
func (client *Client) NewPacket(message string, attr ...interface{}) (*packet, error) {
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
	packet := &packet{Message: message, EventID: uuid4, Project: projectID, ch: make(chan error, 1)}
	packet.MergeAttributes(client.Attributes)
	packet.MergeAttributes(attr)

	if time.Time(packet.Timestamp).IsZero() {
		packet.Timestamp = Timestamp(time.Now())
	}
	if packet.Level == 0 {
		packet.Level = ERROR
	}
	if packet.Logger == "" {
		packet.Logger = "root"
	}
	if packet.ServerName == "" {
		packet.ServerName = ServerName(hostname)
	}
	if packet.Culprit == "" {
		for _, inter := range packet.Interfaces {
			if c, ok := inter.(Culpriter); ok {
				packet.Culprit = c.Culprit()
				if packet.Culprit != "" {
					break
				}
			}
		}
	}

	return packet, nil
}

// Close should be called after a client will no longer be used.
func (client *Client) Close() {
	close(client.queue)
}

// HTTPTransport is the default transport, delivering packets to Sentry via the
// HTTP API.
type HTTPTransport struct {
	http http.Client
}

// Send delivers packets to Sentry via the HTTP API.
func (t *HTTPTransport) Send(url, authHeader string, packet *packet) error {
	if url == "" {
		return nil
	}

	body, contentType := serializedPacket(packet)
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

// serializedPacket converts packets to their optimal transmission encoding.
func serializedPacket(packet *packet) (r io.Reader, contentType string) {
	packetJSON := packet.JSON()

	// Only deflate/base64 the packet if it is bigger than 1KB, as there is
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

// Default system hostname.
var hostname string

func init() {
	hostname, _ = os.Hostname()
}
