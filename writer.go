package raven

type Writer struct {
	Client *Client
	Level  Severity
	Logger string // Logger name reported to Sentry
}

// Write formats the byte slice p into a string, and sends a message to
// Sentry at the severity level indicated by the Writer w.
func (w *Writer) Write(p []byte) (int, error) {
	message := string(p)

	eventInfo := NewEventInfo(message, &Message{message, nil})
	eventInfo.Level = w.Level
	eventInfo.Logger = w.Logger
	w.Client.Capture(eventInfo, nil)

	return len(p), nil
}
