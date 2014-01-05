package raven

type Writer struct {
	Client *Client
	Level  Severity
	Logger string // Logger name reported to Sentry
}

// Write formats the byte slice p into a string, and sends a message to
// Sentry at the severity level indicated by the Writer w.
func (w *Writer) Write(p []byte) (int, error) {
	w.Client.CaptureMessage(string(p), &EventInfo{Level: w.Level, Logger: w.Logger})

	return len(p), nil
}
