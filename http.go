package raven

import (
	"net"
	"net/http"
	"net/url"
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
