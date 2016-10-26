package raven

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

func NewHttp(req *http.Request) *Http {
	proto := "http"
	if req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https" {
		proto = "https"
	}
	h := &Http{
		Method:  req.Method,
		Cookies: req.Header.Get("Cookie"),
		Query:   sanitizeQuery(req.URL.Query()).Encode(),
		URL:     proto + "://" + req.Host + req.URL.Path,
		Headers: make(map[string]string, len(req.Header)),
	}
	if addr, port, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		h.Env = map[string]string{"REMOTE_ADDR": addr, "REMOTE_PORT": port}
	}
	for k, v := range req.Header {
		h.Headers[k] = strings.Join(v, ",")
	}
	return h
}

var querySecretFields = []string{"password", "passphrase", "passwd", "secret"}

func sanitizeQuery(query url.Values) url.Values {
	for _, keyword := range querySecretFields {
		for field := range query {
			if strings.Contains(field, keyword) {
				query[field] = []string{"********"}
			}
		}
	}
	return query
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

// Prints an argument passed to panic.
// There's room for arbitrary complexity here, but we keep it
// simple and handle just a few important cases: int, string, and Stringer.
//
// Taken from runtime/error.go in the standard library (how it prints panics)
func printany(i interface{}) string {
	switch v := i.(type) {
	case nil:
		return "nil"
	case fmt.Stringer:
		return v.String()
	case error:
		return v.Error()
	case int:
		return strconv.Itoa(v)
	case string:
		return v
	default:
		return fmt.Sprintf("%#v", v)
	}
}

// Recovery handler to wrap the stdlib net/http Mux.
// Example:
//	http.HandleFunc("/", raven.RecoveryHandler(func(w http.ResponseWriter, r *http.Request) {
//		...
//	}))
func RecoveryHandler(handler func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rval := recover(); rval != nil {
				os.Stderr.WriteString("panic: ")
				os.Stderr.WriteString(printany(rval) + "\n\n")
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
