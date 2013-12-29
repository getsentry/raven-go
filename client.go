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

var ErrPacketDropped = errors.New("raven: packet dropped")

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
	Send(url, authHeader string, packet *Packet) error
}

type outgoingPacket struct {
	packet *Packet
	ch     chan error
}

// http://sentry.readthedocs.org/en/latest/developer/client/index.html#building-the-json-packet
type Packet struct {
	// Required
	Message string `json:"message"`

	// Required, set automatically by Client.Send/Report via Packet.Init if blank
	EventID   string    `json:"event_id"`
	Project   string    `json:"project"`
	Timestamp Timestamp `json:"timestamp"`
	Level     Severity  `json:"level"`
	Logger    string    `json:"logger"`

	// Optional
	Platform   string                 `json:"platform,omitempty"`
	Culprit    string                 `json:"culprit,omitempty"`
	Tags       map[string]string      `json:"tags,omitempty"`
	ServerName string                 `json:"server_name,omitempty"`
	Modules    []map[string]string    `json:"modules,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`

	Interfaces []Interface `json:"-"`
}

// NewPacket constructs a packet with the specified message and interfaces.
func NewPacket(message string, interfaces ...Interface) *Packet {
	return &Packet{Message: message, Interfaces: interfaces, Tags: make(map[string]string), Extra: make(map[string]interface{})}
}

// Init initializes required fields in a packet. It is typically called by
// Client.Send/Report automatically.
func (packet *Packet) Init(project string, parentTags map[string]string) error {
	if packet.Message == "" {
		return errors.New("raven: empty message")
	}
	if packet.Project == "" {
		packet.Project = project
	}
	if packet.EventID == "" {
		var err error
		packet.EventID, err = randomID()
		if err != nil {
			return nil
		}
	}
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
		packet.ServerName = hostname
	}

	tags := make(map[string]string, len(parentTags)+len(packet.Tags))
	for k, v := range parentTags {
		tags[k] = v
	}
	for k, v := range packet.Tags {
		tags[k] = v
	}
	packet.Tags = tags

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

	return nil
}

func randomID() (string, error) {
	id := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(id), nil
}

func (packet *Packet) JSON() []byte {
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
func NewClient(dsn string, tags map[string]string) (*Client, error) {
	client := &Client{Transport: &HTTPTransport{}, Tags: tags, queue: make(chan *outgoingPacket, MaxQueueBuffer)}
	go client.worker()
	return client, client.SetDSN(dsn)
}

// Client encapsulates a connection to a Sentry server. It must be initialized
// by calling NewClient. Modification of fields concurrently with Send or after
// calling Report for the first time is not thread-safe.
type Client struct {
	Tags map[string]string

	Transport Transport

	// DropHandler is called when a packet is dropped because the buffer is full.
	DropHandler func(*Packet)

	mu         sync.RWMutex
	url        string
	projectID  string
	authHeader string
	queue      chan *outgoingPacket
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
	for outgoingPacket := range client.queue {
		client.mu.RLock()
		url, authHeader := client.url, client.authHeader
		client.mu.RUnlock()

		outgoingPacket.ch <- client.Transport.Send(url, authHeader, outgoingPacket.packet)
	}
}

// Capture asynchronously delivers a packet to the Sentry server. It is a no-op
// when client is nil. A channel is provided if it is important to check for a
// send's success.
func (client *Client) Capture(packet *Packet, captureTags map[string]string) (eventID string, ch chan error) {
	if client == nil {
		return
	}

	ch = make(chan error, 1)

	// Merge client tags and capture tags
	tags := make(map[string]string, len(client.Tags)+len(captureTags))
	for tag, value := range client.Tags {
		tags[tag] = value
	}
	for tag, value := range captureTags {
		tags[tag] = value
	}

	// Initialize any required packet fields
	client.mu.RLock()
	projectID := client.projectID
	client.mu.RUnlock()

	err := packet.Init(projectID, tags)
	if err != nil {
		ch <- err
		return
	}

	outgoingPacket := &outgoingPacket{packet, ch}

	select {
	case client.queue <- outgoingPacket:
	default:
		// Send would block, drop the packet
		if client.DropHandler != nil {
			client.DropHandler(packet)
		}
		ch <- ErrPacketDropped
	}

	return packet.EventID, ch
}

// CaptureMessage formats and delivers a string message to the Sentry server.
func (client *Client) CaptureMessage(message string, tags map[string]string) string {
	packet := NewPacket(message, &Message{message, nil})
	eventID, _ := client.Capture(packet, tags)

	return eventID
}

// CapturePanic formats and delivers recover value information to the Sentry server.
// Adds a stacktrace to the packet, excluding the call to this method.
func (client *Client) CapturePanic(err error, tags map[string]string) string {
	packet := NewPacket(err.Error(), NewException(err, NewStacktrace(1, 3, nil)))
	eventID, _ := client.Capture(packet, tags)

	return eventID
}

// CapturePanics recovers from a yet-unrecovered panic and delivers panic information
// to the Sentry server. Adds a stacktrace to the packet, excluding the overhead of
// this method.
func (client *Client) CapturePanics(tags map[string]string, f func()) {
	defer func() {
		var packet *Packet
		switch rval := recover().(type) {
		case nil:
			return
		case error:
			packet = NewPacket(rval.Error(), NewException(rval, NewStacktrace(2, 3, nil)))
		default:
			rvalStr := fmt.Sprint(rval)
			packet = NewPacket(rvalStr, NewException(errors.New(rvalStr), NewStacktrace(2, 3, nil)))
		}

		client.Capture(packet, tags)
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

func (t *HTTPTransport) Send(url, authHeader string, packet *Packet) error {
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

func serializedPacket(packet *Packet) (r io.Reader, contentType string) {
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

var hostname string

func init() {
	hostname, _ = os.Hostname()
}
