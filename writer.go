package raven

type Writer struct {
	Client *Client
	Level  Severity
}

// Write formats the byte slice p into a string, and sends a message to
// Sentry at the severity level indicated by the Writer w.
func (w *Writer) Write(p []byte) (int, error) {
	message := string(p)

	packet := NewPacket(message, &Message{message, nil})
	packet.Level = w.Level
	err := w.Client.Send(packet)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}
