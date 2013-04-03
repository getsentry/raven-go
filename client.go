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

type Culpriter interface {
	Culprit() string
}

type Transport interface {
	Send(url, authHeader string, packet *Packet) error
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

func NewClient(dsn string, tags map[string]string) (*Client, error) {
	client := &Client{Transport: &HTTPTransport{}, Tags: tags}
	return client, client.SetDSN(dsn)
}

type Client struct {
	Tags map[string]string

	Transport Transport

	mu         sync.RWMutex
	url        string
	projectID  string
	authHeader string
}

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

	client.authHeader = fmt.Sprintf("Sentry sentry_version=3, sentry_key=%s, sentry_secret=%s", publicKey, secretKey)

	return nil
}

func (client *Client) Send(packet *Packet) error {
	client.mu.RLock()
	url, authHeader, projectID := client.url, client.authHeader, client.projectID
	client.mu.RUnlock()

	packet.Init(projectID, client.Tags)
	return client.Transport.Send(url, authHeader, packet)
}

type HTTPTransport struct {
	http http.Client
}

func (t *HTTPTransport) Send(url, authHeader string, packet *Packet) error {
	if url == "" {
		return nil
	}
	req, _ := http.NewRequest("POST", url, bytes.NewReader(packet.JSON()))
	req.Header.Set("X-Sentry-Auth", authHeader)
	req.Header.Set("User-Agent", userAgent)
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
