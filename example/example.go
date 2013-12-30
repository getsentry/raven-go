// You may need to comment some demos out if your Sentry server employs rate limiting!
package main

import (
	"errors"
	"github.com/cupcake/raven-go"
	"log"
	"net/http"
	"os"
	"time"
)

var LunarLanding = time.Date(1969, 7, 20, 20, 17, 40, 0, time.UTC)

func trace() *raven.Stacktrace {
	return raven.NewStacktrace(0, 2, nil)
}

func main() {
	client, err := raven.NewClient(os.Args[1], map[string]string{"foo": "bar"})
	if err != nil {
		log.Fatal(err)
	}

	// Demo plain message
	client.CaptureMessage("Just a message")

	// Demo plain error
	client.CaptureError(errors.New("Just an error"))

	// Demo message with Demo logging level
	client.CaptureMessage("Just a warning message", raven.Severity(raven.WARNING))

	// Demo message with lots of custom stuff. Note: you'll have to change your date filters in Sentry to see this!
	client.CaptureMessage("A message with a lot of stuff",
		raven.Severity(raven.INFO),
		raven.Logger("com.cupcake.raven-go.stuff-demo"),
		raven.Timestamp(LunarLanding),
		raven.Culprit("Moon Beams"),
	)

	// Demo HTTP interface
	httpReq, _ := http.NewRequest("GET", "http://example.com/foo?bar=true", nil)
	httpReq.RemoteAddr = "127.0.0.1:80"
	httpReq.Header = http.Header{"Content-Type": {"text/html"}, "Content-Length": {"42"}}
	_, ch := client.Capture("Test report", raven.NewException(errors.New("example"), trace()), raven.NewHttp(httpReq))
	if err = <-ch; err != nil {
		log.Fatal(err)
	}

	// Demo multiple interfaces
	_, ch = client.Capture("Test Query",
		raven.Severity(raven.INFO),
		raven.Logger("com.cupcake.raven-go.db-logger"),
		raven.Query{"SELECT * FROM tests;", "postgres"},
		raven.User{"44", "titanous", "titanous@example.com"},
		trace(),
	)
	if err = <-ch; err != nil {
		log.Fatal(err)
	}
}
