package raven

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// TestEvent_JSON validates an Event with most fields set serializes to correct JSON.
func TestEvent_JSON(t *testing.T) {
	event := &Event{
		Message:    "test",
		EventId:    "2",
		Project:    "1",
		Timestamp:  Timestamp(time.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC)),
		Level:      Error,
		Logger:     "com.getsentry.raven-go.test-logger",
		Platform:   "go",
		Culprit:    "TestEvent_JSON",
		ServerName: "test.getsentry.com",
		Tags:       []Tag{Tag{"foo", "bar"}, Tag{"foo", "foo"}, Tag{"baz", "buzz"}},
		Interfaces: []Interface{&Message{Message: "foo"}},
	}

	expected := `{"message":"test","event_id":"2","project":"1","timestamp":"2000-01-01T00:00:00","level":"error","logger":"com.getsentry.raven-go.test-logger","platform":"go","culprit":"TestEvent_JSON","tags":[["foo","bar"],["foo","foo"],["baz","buzz"]],"server_name":"test.getsentry.com","sentry.interfaces.Message":{"message":"foo"}}`
	actual := string(event.JSON())

	if actual != expected {
		t.Errorf("incorrect json; got %s, want %s", actual, expected)
	}
}

// TestEvent_FillDefaults verifies that all required and possible nice-to-have fields
// get populated after Event.FillDefaults.
func TestEvent_FillDefaults(t *testing.T) {
	event := &Event{Message: "a", Interfaces: []Interface{&testInterface{}}}
	event.FillDefaults("foo")

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

func TestTag_UnmarshalJSON(t *testing.T) {
	actual := new(Tag)
	if err := json.Unmarshal([]byte(`["foo","bar"]`), actual); err != nil {
		t.Fatal("unable to decode JSON:", err)
	}

	expected := &Tag{Key: "foo", Value: "bar"}
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("incorrect Tag: wanted '%+v' and got '%+v'", expected, actual)
	}
}

func TestTags_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		Input    string
		Expected Tags
	}{
		{
			`{"foo":"bar","bar":"baz"}`,
			Tags{Tag{Key: "foo", Value: "bar"}, Tag{Key: "bar", Value: "baz"}},
		},
		{
			`[["foo","bar"],["bar","baz"]]`,
			Tags{Tag{Key: "foo", Value: "bar"}, Tag{Key: "bar", Value: "baz"}},
		},
	}

	for _, test := range tests {
		var actual Tags
		if err := json.Unmarshal([]byte(test.Input), &actual); err != nil {
			t.Fatal("unable to decode JSON:", err)
		}

		if !reflect.DeepEqual(actual, test.Expected) {
			t.Errorf("incorrect Tags: wanted '%+v' and got '%+v'", test.Expected, actual)
		}
	}
}

func TestTimestamp_MarshalJSON(t *testing.T) {
	timestamp := Timestamp(time.Date(2000, 01, 02, 03, 04, 05, 0, time.UTC))
	expected := `"2000-01-02T03:04:05"`

	actual, err := json.Marshal(timestamp)
	if err != nil {
		t.Error(err)
	}

	if string(actual) != expected {
		t.Errorf("incorrect string; got %s, want %s", actual, expected)
	}
}

func TestTimestamp_UnmarshalJSON(t *testing.T) {
	timestamp := `"2000-01-02T03:04:05"`
	expected := Timestamp(time.Date(2000, 01, 02, 03, 04, 05, 0, time.UTC))

	var actual Timestamp
	err := json.Unmarshal([]byte(timestamp), &actual)
	if err != nil {
		t.Error(err)
	}

	if actual != expected {
		t.Errorf("incorrect string; got %s, want %s", actual, expected)
	}
}
