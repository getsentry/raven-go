package raven

import (
	"reflect"
	"regexp"
)

// http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Message
type Message struct {
	// Required
	Message string `json:"message"`

	// Optional
	Params []interface{} `json:"params,omitempty"`
}

func (m *Message) Class() string { return "sentry.interfaces.Message" }

// http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Exception
type Exception struct {
	// Required
	Value string `json:"value"`

	// Optional
	Type       string      `json:"type,omitempty"`
	Module     string      `json:"module,omitempty"`
	Stacktrace *Stacktrace `json:"stacktrace,omitempty"`
}

var errorMsgPattern = regexp.MustCompile(`\A(\w+): (.+)\z`)

func NewException(err error, stacktrace *Stacktrace) *Exception {
	msg := err.Error()
	ex := &Exception{
		Stacktrace: stacktrace,
		Value:      msg,
		Type:       reflect.TypeOf(err).String(),
	}
	if m := errorMsgPattern.FindStringSubmatch(msg); m != nil {
		ex.Module, ex.Value = m[1], m[2]
	}
	return ex
}

func (e *Exception) Class() string { return "sentry.interfaces.Exception" }

func (e *Exception) Culprit() string {
	if e.Stacktrace == nil {
		return ""
	}
	return e.Stacktrace.Culprit()
}

// http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Template
type Template struct {
	// Required
	Filename    string `json:"filename"`
	Lineno      int    `json:"lineno"`
	ContextLine string `json:"context_line"`

	// Optional
	PreContext   []string `json:"pre_context,omitempty"`
	PostContext  []string `json:"post_context,omitempty"`
	AbsolutePath string   `json:"abs_path,omitempty"`
}

func (t *Template) Class() string { return "sentry.interfaces.Template" }

// http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.User
type User struct {
	ID       string `json:"id"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
}

func (h *User) Class() string { return "sentry.interfaces.User" }

// http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Query
type Query struct {
	// Required
	Query string `json:"query"`

	// Optional
	Engine string `json:"engine,omitempty"`
}

func (q *Query) Class() string { return "sentry.interfaces.Query" }
