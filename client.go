package raven

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const userAgent = "go-raven/1.0"

type Severity int

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

// http://sentry.readthedocs.org/en/latest/developer/client/index.html#building-the-json-packet
type Packet struct {
	// Required
	Message string `json:"message"`

	// Required, set automatically by Client.Send via Packet.Init if blank
	EventID   string    `json:"event_id"`
	Project   string    `json:"project"`
	Timestamp Timestamp `json:"timestamp"`
	Level     Severity  `json:"level"`

	// Optional
	Logger     string                 `json:"logger,omitempty"`
	Platform   string                 `json:"platform,omitempty"`
	Culprit    string                 `json:"culprit,omitempty"`
	Tags       map[string]string      `json:"tags,omitempty"`
	ServerName string                 `json:"server_name,omitempty"`
	Modules    []map[string]string    `json:"modules,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`

	Interfaces []Interface `json:"-"`
}

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

	tags := make(map[string]string)
	for k, v := range parentTags {
		tags[k] = v
	}
	for k, v := range packet.Tags {
		tags[k] = v
	}
	packet.Tags = tags

	return nil
}

func (packet *Packet) JSON() []byte {
	packetJSON, _ := json.Marshal(packet)

	interfaces := make(map[string]Interface)
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

func NewClient(dsn string) (*Client, error) {
	client := &Client{}
	return client, client.SetDSN(dsn)
}

type Client struct {
	mu sync.RWMutex

	url string

	publicKey string
	secretKey string
	projectID string

	tags map[string]string

	authHeader string

	http http.Client
}

func (client *Client) SetDSN(dsn string) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	uri, err := url.Parse(dsn)
	if err != nil {
		return err
	}

	if uri.User == nil {
		return errors.New("raven: dsn missing public key and/or private key")
	}
	client.publicKey = uri.User.Username()
	var ok bool
	if client.secretKey, ok = uri.User.Password(); !ok {
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

	client.authHeader = fmt.Sprintf("Sentry sentry_version=3, sentry_key=%s, sentry_secret=%s", client.publicKey, client.secretKey)

	return nil
}

func (client *Client) SetTags(tags map[string]string) {
	client.mu.Lock()
	client.tags = tags
	client.mu.Unlock()
}

func (client *Client) Send(packet *Packet) error {
	client.mu.RLock()
	packet.Init(client.projectID, client.tags)
	req, _ := http.NewRequest("POST", client.url, bytes.NewReader(packet.JSON()))
	req.Header.Set("X-Sentry-Auth", client.authHeader)
	req.Header.Set("User-Agent", userAgent)
	client.mu.RUnlock()
	res, err := client.http.Do(req)
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

func randomID() (string, error) {
	id := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, id)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(id), nil
}
