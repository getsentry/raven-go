package raven

import (
	"net/http"
	"net/url"
	"strings"
)

// Http is a Sentry Interface for reporting HTTP requests. All Http must be created using
// NewHttp.
//
// See http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Http
// for more discussion of this interface.
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

// NewHttp creates a new Sentry HTTP Interface.
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
	if addr := strings.SplitN(req.RemoteAddr, ":", 2); len(addr) == 2 {
		h.Env = map[string]string{"REMOTE_ADDR": addr[0], "REMOTE_PORT": addr[1]}
	}
	for k, v := range req.Header {
		h.Headers[k] = strings.Join(v, "; ")
	}
	return h
}

// Class reports the Sentry HTTP Interface class.
func (h *Http) Class() string { return "sentry.interfaces.Http" }

var querySecretFields = []string{"password", "passphrase", "passwd", "secret"}

// sanitizeQuery does basic sanitization of query values.
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
