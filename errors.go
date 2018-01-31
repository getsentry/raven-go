package raven

type errWrappedWithExtra struct {
	err       error
	extraInfo map[string]interface{}
}

func (ewx *errWrappedWithExtra) Error() string {
	return ewx.err.Error()
}

func (ewx *errWrappedWithExtra) Source() error {
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
