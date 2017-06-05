package raven

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
)

const (
	redaction = "********"
)

// Query fields whose value will redacted. Used by NewHttp.
var QuerySecretFields = []string{"password", "passphrase", "passwd", "secret"}

// Header fields whose value will redacted. Used by NewHttp.
var HeaderSecretFields = []string{"Authorization"}

func redactQuery(r *http.Request) string {
	query := r.URL.Query()

	for _, keyword := range QuerySecretFields {
		for field := range query {
			if field == keyword {
				query[field] = []string{redaction}
				break
			}
		}
	}

	return query.Encode()
}

func redactHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string, len(r.Header))

	for k, v := range r.Header {
		for _, field := range HeaderSecretFields {
			if field == k {
				rep := strings.Repeat(redaction+",", len(v))
				headers[k] = rep[:len(rep)-1]
				break
			}
			headers[k] = strings.Join(v, ",")
		}
	}

	return headers
}

func NewHttp(req *http.Request) *Http {
	proto := "http"
	if req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https" {
		proto = "https"
	}
	h := &Http{
		Method:  req.Method,
		Cookies: req.Header.Get("Cookie"),
		Query:   redactQuery(req),
		URL:     proto + "://" + req.Host + req.URL.Path,
		Headers: redactHeaders(req),
	}
	if addr, port, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		h.Env = map[string]string{"REMOTE_ADDR": addr, "REMOTE_PORT": port}
	}

	return h
}

// https://docs.getsentry.com/hosted/clientdev/interfaces/#context-interfaces
type Http struct {
	// Required
	URL    string `json:"url"`
	Method string `json:"method"`
	Query  string `json:"query_string,omitempty"`

	// Optional
	Cookies string            `json:"cookies,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// Must be either a string or map[string]string
	Data interface{} `json:"data,omitempty"`
}

func (h *Http) Class() string { return "request" }

// Recovery handler to wrap the stdlib net/http Mux.
// Example:
//	http.HandleFunc("/", raven.RecoveryHandler(func(w http.ResponseWriter, r *http.Request) {
//		...
//	}))
func RecoveryHandler(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rval := recover(); rval != nil {
				debug.PrintStack()
				rvalStr := fmt.Sprint(rval)
				packet := NewPacket(rvalStr, NewException(errors.New(rvalStr), NewStacktrace(2, 3, nil)), NewHttp(r))
				Capture(packet, nil)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()

		handler(w, r)
	}
}
