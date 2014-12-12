package raven

import (
	"testing"
	"time"
)

type testInterface struct{}

func (t *testInterface) Class() string   { return "sentry.interfaces.Test" }
func (t *testInterface) Culprit() string { return "codez" }

func TestEventJSON(t *testing.T) {
	event := &Event{
		Project:    "1",
		EventID:    "2",
		Message:    "test",
		Timestamp:  Timestamp(time.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC)),
		Level:      ERROR,
		Logger:     "com.cupcake.raven-go.logger-test-event-json",
		Tags:       []Tag{Tag{"foo", "bar"}, Tag{"foo", "foo"}, Tag{"baz", "buzz"}},
		Interfaces: []Interface{&Message{Message: "foo"}},
	}

	expected := `{"message":"test","event_id":"2","project":"1","timestamp":"2000-01-01T00:00:00","level":40,"logger":"com.cupcake.raven-go.logger-test-event-json","tags":[["foo","bar"],["foo","foo"],["baz","buzz"]],"sentry.interfaces.Message":{"message":"foo"}}`
	actual := string(event.JSON())

	if actual != expected {
		t.Errorf("incorrect json; got %s, want %s", actual, expected)
	}
}

func TestFillEventDefaults(t *testing.T) {
	event := &Event{Message: "a", Interfaces: []Interface{&testInterface{}}}
	event.FillDefaults("foo")

	if len(event.EventID) != 32 {
		t.Error("incorrect EventID:", event.EventID)
	}
	if event.Project != "foo" {
		t.Error("incorrect Project:", event.Project)
	}
	if time.Time(event.Timestamp).IsZero() {
		t.Error("Timestamp is zero")
	}
	if event.Level != ERROR {
		t.Errorf("incorrect Level: got %d, want %d", event.Level, ERROR)
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

func TestSetDSN(t *testing.T) {
	client := &Client{}
	client.SetDSN("https://u:p@example.com/sentry/1")

	if client.url != "https://example.com/sentry/api/1/store/" {
		t.Error("incorrect url:", client.url)
	}
	if client.projectID != "1" {
		t.Error("incorrect projectID:", client.projectID)
	}
	if client.authHeader != "Sentry sentry_version=4, sentry_key=u, sentry_secret=p" {
		t.Error("incorrect authHeader:", client.authHeader)
	}
}

func TestFormatEventID(t *testing.T) {
	formatted := FormatEventID("f47ac10b58cc4372a5670e02b2c3d479")
	expected := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	if formatted != expected {
		t.Errorf("incorrect uuid: expected %s, got %s", expected, formatted)
	}
}
