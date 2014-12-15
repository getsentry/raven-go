package raven

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

type Transport interface {
	Send(url, authHeader string, event *Event) error
}

// HTTPTransport is the default transport, delivering events to Sentry via the
// HTTP API.
type HTTPTransport struct {
	Http http.Client
}

func (t *HTTPTransport) Send(url, authHeader string, event *Event) error {
	if url == "" {
		return nil
	}

	body, contentType := event.serialize()
	req, _ := http.NewRequest("POST", url, body)
	req.Header.Set("X-Sentry-Auth", authHeader)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", contentType)
	res, err := t.Http.Do(req)
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
