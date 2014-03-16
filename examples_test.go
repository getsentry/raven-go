package raven

import (
	"errors"
	"fmt"
	"net/http"
	"log"
	"os"
)

func Example() {
	// ... i.e. raisedErr is incoming error
	raisedErr := errors.New("error message here")
	sentryDSN := "sentry dsn here"
	r, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		log.Fatal(err)
	}
	client, err := NewClient(sentryDSN, map[string]string{})
	if err != nil {
		log.Fatal(err)
	}
	packet := NewPacket(raisedErr.Error(), NewException(raisedErr, trace()), NewHttp(r))
	eventID, ch := client.Capture(packet, nil)
	if err = <-ch; err != nil {
		log.Fatal(err)
	}
	message := fmt.Sprintf("Error event with id \"%s\" - %s", eventID, raisedErr.Error())
	log.Println(message)
}

func ExamplePacket() {
	// os.Args[1] is sentry DSN string
	client, err := NewClient(os.Args[1], map[string]string{"foo": "bar"})
	if err != nil {
		log.Fatal(err)
	}
	req, _ := http.NewRequest("GET", "http://example.com/foo?bar=true", nil)
	req.RemoteAddr = "127.0.0.1:80"
	req.Header = http.Header{"Content-Type": {"text/html"}, "Content-Length": {"42"}}
	packet := &Packet{Message: "Test report", Interfaces: []Interface{NewException(errors.New("example"), trace()), NewHttp(req)}}
	_, ch := client.Capture(packet, nil)
	if err = <-ch; err != nil {
		log.Fatal(err)
	}
	log.Print("sent packet successfully")
}
