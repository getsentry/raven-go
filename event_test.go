package raven

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// TODO: TestEvent_Fill is unimplemented.

// TestEvent_json validates an Event with most fields set serializes to correct JSON.
func TestEvent_json(t *testing.T) {
	event := &Event{
		Message:    "test",
		EventId:    "2",
		Project:    "1",
		Timestamp:  Timestamp(time.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC)),
		Level:      Error,
		Logger:     "com.getsentry.raven-go.test-logger",
		Platform:   "go",
		Culprit:    "TestEvent_json",
		ServerName: "test.getsentry.com",
		Tags:       []Tag{Tag{"foo", "bar"}, Tag{"foo", "foo"}, Tag{"baz", "buzz"}},
		Interfaces: []Interface{&Message{Message: "foo"}},
	}

	expected := `{"message":"test","event_id":"2","project":"1","timestamp":"2000-01-01T00:00:00","level":"error","logger":"com.getsentry.raven-go.test-logger","platform":"go","culprit":"TestEvent_json","tags":[["foo","bar"],["foo","foo"],["baz","buzz"]],"server_name":"test.getsentry.com","sentry.interfaces.Message":{"message":"foo"}}`
	actual := string(event.json())

	if actual != expected {
		t.Errorf("incorrect json; got %s, want %s", actual, expected)
	}
}

// TestTag_UnmarshalJSON validates tags deserialize from JSON properly.
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

// TestTags_UnmarshalJSON validates both kinds of tags deserialize from JSON properly.
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

// TestTimestamp_MarshalJSON validates timestamps serialize into the type of timestamp
// Sentry expects.
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

// TestTimestamp_UnmarshalJSON validates Sentry timestamps can be deserialized into
// a Timestamp.
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
