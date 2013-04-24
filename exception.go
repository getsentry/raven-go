package raven

import (
	"reflect"
	"regexp"
)

var errorMsgPattern = regexp.MustCompile(`\A(\w+): (.+)\z`)

func NewException(err error) *Exception {
	msg := err.Error()
	ex := &Exception{
		Value: msg,
		Type:  reflect.TypeOf(err).String(),
	}
	if m := errorMsgPattern.FindStringSubmatch(msg); m != nil {
		ex.Module, ex.Value = m[1], m[2]
	}
	return ex
}

// http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Exception
type Exception struct {
	// Required
	Value string `json:"value"`

	// Optional
	Type   string `json:"type,omitempty"`
	Module string `json:"module,omitempty"`
}

func (e *Exception) Class() string { return "sentry.interfaces.Exception" }
