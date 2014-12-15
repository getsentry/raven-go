package main

import (
	"errors"
	"fmt"
	"github.com/getsentry/raven-go"
	"log"
	"net/http"
	"os"
)

func trace() *raven.Stacktrace {
	return raven.NewStacktrace(0, 2, nil)
}

func main() {
	client, err := raven.NewClient(os.Args[1], &raven.EventInfo{Tags: []raven.Tag{raven.Tag{"foo", "bar"}}})
	if err != nil {
		log.Fatalln(err)
	}

	httpReq, _ := http.NewRequest("GET", "http://example.com/foo?bar=true", nil)
	httpReq.RemoteAddr = "127.0.0.1:80"
	httpReq.Header = http.Header{"Content-Type": {"text/html"}, "Content-Length": {"42"}}

	event := &raven.EventInfo{Interfaces: []raven.Interface{raven.NewException(errors.New("example"), trace()), raven.NewHttp(httpReq)}}
	eventID, ch := client.Capture("Test report", event)
	if err = <-ch; err != nil {
		log.Fatalln(err)
	}

	log.Println("sent event successfully:", raven.FormatEventID(eventID))
}

// CheckError sends error report to sentry and records event id and error name to the logs
func CheckError(err error, r *http.Request) {
	client, err := raven.NewClient(os.Args[1], map[string]string{"foo": "bar"})
	if err != nil {
		log.Fatal(err)
	}
	packet := raven.NewPacket(err.Error(), raven.NewException(err, trace()), raven.NewHttp(r))
	eventID, _ := client.Capture(packet, nil)
	message := fmt.Sprintf("Error event with id \"%s\" - %s", eventID, err.Error())
	log.Println(message)
}
