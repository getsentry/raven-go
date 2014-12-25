package raven

// Writer is designed to be used with log.New to create a new log.Logger that logs to
// Sentry.
type Writer struct {
	Client *Client
	Level  Severity
	Logger string // Logger name reported to Sentry
}

// Write formats the byte slice p into a string, and sends a message to
// Sentry at the severity level indicated by the Writer w.
func (w *Writer) Write(p []byte) (int, error) {
	w.Client.CaptureMessage(string(p), &Context{Level: w.Level, Logger: w.Logger})

	return len(p), nil
}
