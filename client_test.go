package raven

import (
	"testing"
	"time"
)

// TestClient_SetDSN verifies the DSN is successfully parsed and applied to Client.
func TestClient_SetDSN(t *testing.T) {
	client := &Client{}
	client.SetDSN("https://u:p@example.com/sentry/1")

	if client.url != "https://example.com/sentry/api/1/store/" {
		t.Error("incorrect url:", client.url)
	}
	if client.projectId != "1" {
		t.Error("incorrect projectId:", client.projectId)
	}
	if client.authHeader != "Sentry sentry_version=4, sentry_key=u, sentry_secret=p" {
		t.Error("incorrect authHeader:", client.authHeader)
	}
}

// TestClient_finalizeEvent verifies that all required and possible nice-to-have fields
// get populated after Event.FillDefaults.
func TestClient_finalizeEvent(t *testing.T) {
	client := &Client{context: &Context{}, projectId: "foo"}
	event := &Event{Message: "a", Interfaces: []Interface{&testInterface{}}}
	client.finalizeEvent(event, nil)

	if len(event.EventId) != 32 {
		t.Error("incorrect EventId:", event.EventId)
	}
	if event.Project != "foo" {
		t.Error("incorrect Project:", event.Project)
	}
	if time.Time(event.Timestamp).IsZero() {
		t.Error("Timestamp is zero")
	}
	if event.Level != Error {
		t.Errorf("incorrect Level: got %d, want %d", event.Level, Error)
	}
	if event.Logger != "root" {
		t.Errorf("incorrect Logger: got %s, want %s", event.Logger, "root")
	}
	if event.ServerName == "" {
		t.Errorf("ServerName should not be empty")
	}
	if event.Culprit != "codez" {
		t.Error("incorrect Culprit:", event.Culprit)
	}
}
