package logrus_sentry

import (
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/getsentry/raven-go"
)

const (
	message     = "error message"
	server_name = "testserver.internal"
	logger_name = "test.logger"
)

func getTestLogger() *logrus.Logger {
	l := logrus.New()
	l.Out = ioutil.Discard
	return l
}

// raven.Packet does not have a json directive for deserializing stacktrace
// so need to explicitly construct one for purpose of test
type resultPacket struct {
	raven.Packet
	Stacktrace raven.Stacktrace `json:stacktrace`
}

func WithTestDSN(t *testing.T, tf func(string, <-chan *resultPacket)) {
	pch := make(chan *resultPacket, 1)
	s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		contentType := req.Header.Get("Content-Type")
		var bodyReader io.Reader = req.Body
		// underlying client will compress and encode payload above certain size
		if contentType == "application/octet-stream" {
			bodyReader = base64.NewDecoder(base64.StdEncoding, bodyReader)
			bodyReader, _ = zlib.NewReader(bodyReader)
		}

		d := json.NewDecoder(bodyReader)
		p := &resultPacket{}
		err := d.Decode(p)
		if err != nil {
			t.Fatal(err.Error())
		}

		pch <- p
	}))
	defer s.Close()

	fragments := strings.SplitN(s.URL, "://", 2)
	dsn := fmt.Sprintf(
		"%s://public:secret@%s/sentry/project-id",
		fragments[0],
		fragments[1],
	)
	tf(dsn, pch)
}

func TestSpecialFields(t *testing.T) {
	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		logger := getTestLogger()

		hook, err := NewSentryHook(dsn, []logrus.Level{
			logrus.ErrorLevel,
		})

		if err != nil {
			t.Fatal(err.Error())
		}
		logger.Hooks.Add(hook)

		req, _ := http.NewRequest("GET", "url", nil)
		logger.WithFields(logrus.Fields{
			"server_name":  server_name,
			"logger":       logger_name,
			"http_request": req,
		}).Error(message)

		packet := <-pch
		if packet.Logger != logger_name {
			t.Errorf("logger should have been %s, was %s", logger_name, packet.Logger)
		}

		if packet.ServerName != server_name {
			t.Errorf("server_name should have been %s, was %s", server_name, packet.ServerName)
		}
	})
}

func TestSentryHandler(t *testing.T) {
	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		logger := getTestLogger()
		hook, err := NewSentryHook(dsn, []logrus.Level{
			logrus.ErrorLevel,
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		logger.Hooks.Add(hook)

		logger.Error(message)
		packet := <-pch
		if packet.Message != message {
			t.Errorf("message should have been %s, was %s", message, packet.Message)
		}
	})
}

func TestSentryWithClient(t *testing.T) {
	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		logger := getTestLogger()

		client, _ := raven.New(dsn)

		hook, err := NewWithClientSentryHook(client, []logrus.Level{
			logrus.ErrorLevel,
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		logger.Hooks.Add(hook)

		logger.Error(message)
		packet := <-pch
		if packet.Message != message {
			t.Errorf("message should have been %s, was %s", message, packet.Message)
		}
	})
}

func TestSentryTags(t *testing.T) {
	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		logger := getTestLogger()
		tags := map[string]string{
			"site": "test",
		}
		levels := []logrus.Level{
			logrus.ErrorLevel,
		}

		hook, err := NewWithTagsSentryHook(dsn, tags, levels)
		if err != nil {
			t.Fatal(err.Error())
		}

		logger.Hooks.Add(hook)

		logger.Error(message)
		packet := <-pch
		expected := raven.Tags{
			raven.Tag{
				Key:   "site",
				Value: "test",
			},
		}
		if !reflect.DeepEqual(packet.Tags, expected) {
			t.Errorf("message should have been %s, was %s", message, packet.Message)
		}
	})
}

func TestSentryStacktrace(t *testing.T) {
	WithTestDSN(t, func(dsn string, pch <-chan *resultPacket) {
		logger := getTestLogger()
		hook, err := NewSentryHook(dsn, []logrus.Level{
			logrus.ErrorLevel,
			logrus.InfoLevel,
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		logger.Hooks.Add(hook)

		logger.Error(message)
		packet := <-pch
		stacktraceSize := len(packet.Stacktrace.Frames)
		if stacktraceSize != 0 {
			t.Error("Stacktrace should be empty as it is not enabled")
		}

		hook.StacktraceConfiguration.Enable = true

		logger.Error(message) // this is the call that the last frame of stacktrace should capture
		expectedLineno := 195 //this should be the line number of the previous line

		packet = <-pch
		stacktraceSize = len(packet.Stacktrace.Frames)
		if stacktraceSize == 0 {
			t.Error("Stacktrace should not be empty")
		}
		lastFrame := packet.Stacktrace.Frames[stacktraceSize-1]
		expectedSuffix := "logrus_sentry/sentry_test.go"
		if !strings.HasSuffix(lastFrame.Filename, expectedSuffix) {
			t.Errorf("File name should have ended with %s, was %s", expectedSuffix, lastFrame.Filename)
		}
		if lastFrame.Lineno != expectedLineno {
			t.Errorf("Line number should have been %s, was %s", expectedLineno, lastFrame.Lineno)
		}
		if lastFrame.InApp {
			t.Error("Frame should not be identified as in_app without prefixes")
		}

		hook.StacktraceConfiguration.InAppPrefixes = []string{"github.com/Sirupsen/logrus"}
		hook.StacktraceConfiguration.Context = 2
		hook.StacktraceConfiguration.Skip = 2

		logger.Error(message)
		packet = <-pch
		stacktraceSize = len(packet.Stacktrace.Frames)
		if stacktraceSize == 0 {
			t.Error("Stacktrace should not be empty")
		}
		lastFrame = packet.Stacktrace.Frames[stacktraceSize-1]
		expectedFilename := "github.com/Sirupsen/logrus/entry.go"
		if lastFrame.Filename != expectedFilename {
			t.Errorf("File name should have been %s, was %s", expectedFilename, lastFrame.Filename)
		}
		if !lastFrame.InApp {
			t.Error("Frame should be identified as in_app")
		}
	})
}
