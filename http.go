package raven

import (
	"net/http"
	"strings"
)

func NewHttp(req *http.Request) *Http {
	// TODO: sanitization
	// TODO: detect http vs https
	h := &Http{
		Method:  req.Method,
		Cookies: req.Header.Get("Cookie"),
		Query:   req.URL.RawQuery,
		URL:     "http://" + req.Host + req.URL.Path,
		Headers: make(map[string]string),
	}
	if addr := strings.SplitN(req.RemoteAddr, ":", 2); len(addr) == 2 {
		h.Env = map[string]string{"REMOTE_ADDR": addr[0], "REMOTE_PORT": addr[1]}
	}
	for k, v := range req.Header {
		h.Headers[k] = strings.Join(v, "; ")
	}
	return h
}

// http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Http
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

func (h *Http) Class() string { return "sentry.interfaces.Http" }
