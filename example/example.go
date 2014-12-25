package main

import (
	"errors"
	"github.com/getsentry/raven-go"
	"log"
	"net/http"
	"os"
)

func trace() *raven.Stacktrace {
	return raven.NewStacktrace(0, 2, nil)
}

func main() {
	client, err := raven.NewClient(os.Args[1], &raven.Context{Tags: []raven.Tag{raven.Tag{"foo", "bar"}}})
	if err != nil {
		log.Fatalln(err)
	}

	httpReq, _ := http.NewRequest("GET", "http://example.com/foo?bar=true", nil)
	httpReq.RemoteAddr = "127.0.0.1:80"
	httpReq.Header = http.Header{"Content-Type": {"text/html"}, "Content-Length": {"42"}}

	context := &raven.Context{Interfaces: []raven.Interface{raven.NewException(errors.New("example"), trace()), raven.NewHttp(httpReq)}}
	eventId, ch := client.CaptureMessage("Test report", context)
	if err = <-ch; err != nil {
		log.Fatalln(err)
	}

	log.Println("sent event successfully:", eventId)
}
