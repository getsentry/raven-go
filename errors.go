package raven

import pkgErrors "github.com/pkg/errors"

type causer interface {
	Cause() error
}

type stacktracer interface {
	StackTrace() pkgErrors.StackTrace
}

type errWrappedWithExtra struct {
	err       error
	extraInfo map[string]interface{}
}

func (ewx *errWrappedWithExtra) Error() string {
	return ewx.err.Error()
}

func (ewx *errWrappedWithExtra) Cause() error {
	return ewx.err
}

func (ewx *errWrappedWithExtra) ExtraInfo() Extra {
	return ewx.extraInfo
}

func WrapWithExtra(err error, extraInfo map[string]interface{}) error {
	return &errWrappedWithExtra{
		err:       err,
		extraInfo: extraInfo,
	}
}

type ErrWithExtra interface {
	Error() string
	Cause() error
	ExtraInfo() Extra
}

func extractExtra(err error) Extra {
	extra := Extra{}

	currentErr := err
	for currentErr != nil {
		if errWithExtra, ok := currentErr.(ErrWithExtra); ok {
			for k, v := range errWithExtra.ExtraInfo() {
				extra[k] = v
			}
		}

		if errWithCause, ok := currentErr.(causer); ok {
			currentErr = errWithCause.Cause()
		} else {
			currentErr = nil
		}
	}

	return extra
}
