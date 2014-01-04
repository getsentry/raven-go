package raven

import (
	"testing"
	"time"
)

type testInterface struct{}

func (t *testInterface) Class() string   { return "sentry.interfaces.Test" }
func (t *testInterface) Culprit() string { return "codez" }

func TestPacketJSON(t *testing.T) {
	eventInfo := &EventInfo{
		Project:    "1",
		EventID:    "2",
		Message:    "test",
		Timestamp:  Timestamp(time.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC)),
		Level:      ERROR,
		Logger:     "com.cupcake.raven-go.logger-test-eventInfo-json",
		Tags:       []Tag{Tag{"foo", "bar"}},
		Interfaces: []Interface{&Message{Message: "foo"}},
	}

	eventInfo.AddTags(map[string]string{"foo": "foo", "baz": "buzz"})

	expected := `{"message":"test","event_id":"2","project":"1","timestamp":"2000-01-01T00:00:00","level":40,"logger":"com.cupcake.raven-go.logger-test-eventInfo-json","tags":[["foo","bar"],["foo","foo"],["baz","buzz"]],"sentry.interfaces.Message":{"message":"foo"}}`
	actual := string(eventInfo.JSON())

	if actual != expected {
		t.Errorf("incorrect json; got %s, want %s", actual, expected)
	}
}

func TestPacketInit(t *testing.T) {
	eventInfo := &EventInfo{Message: "a", Interfaces: []Interface{&testInterface{}}}
	eventInfo.Init("foo")

	if eventInfo.Project != "foo" {
		t.Error("incorrect Project:", eventInfo.Project)
	}
	if eventInfo.Culprit != "codez" {
		t.Error("incorrect Culprit:", eventInfo.Culprit)
	}
	if eventInfo.ServerName == "" {
		t.Errorf("ServerName should not be empty")
	}
	if eventInfo.Level != ERROR {
		t.Errorf("incorrect Level: got %d, want %d", eventInfo.Level, ERROR)
	}
	if eventInfo.Logger != "root" {
		t.Errorf("incorrect Logger: got %s, want %s", eventInfo.Logger, "root")
	}
	if time.Time(eventInfo.Timestamp).IsZero() {
		t.Error("Timestamp is zero")
	}
	if len(eventInfo.EventID) != 32 {
		t.Error("incorrect EventID:", eventInfo.EventID)
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

func TestFormatUUID(t *testing.T) {
	formatted := FormatUUID("f47ac10b58cc4372a5670e02b2c3d479")
	expected := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	if formatted != expected {
		t.Errorf("incorrect uuid: expected %s, got %s", expected, formatted)
	}
}
