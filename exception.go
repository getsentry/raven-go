package raven

import (
	"reflect"
	"regexp"
)

var errorMsgPattern = regexp.MustCompile(`\A(\w+): (.+)\z`)

// Exception is a Sentry Interface for reporting exceptions. It is used in raven-go to
// report errors.
//
// All Exceptions must be created using NewException.
// See http://sentry.readthedocs.org/en/latest/developer/interfaces/index.html#sentry.interfaces.Exception
// for more discussion of this interface.
type Exception struct {
	// Required
	Value string `json:"value"`

	// Optional
	Type       string      `json:"type,omitempty"`
	Module     string      `json:"module,omitempty"`
	Stacktrace *Stacktrace `json:"stacktrace,omitempty"`
}

// NewException creates a new Sentry Exception Interface.
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

// Class reports the Sentry Exception Interface class.
func (e *Exception) Class() string { return "sentry.interfaces.Exception" }

// Culprit derives an exception's culprit from its stack trace.
func (e *Exception) Culprit() string {
	if e.Stacktrace == nil {
		return ""
	}
	return e.Stacktrace.Culprit()
}
