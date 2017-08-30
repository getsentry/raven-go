package raven

import (
	"fmt"
	"log"
	"net/http"
)

func Example() {
	// ... i.e. raisedErr is incoming error
	var raisedErr error
	// sentry DSN generated by Sentry server
	var sentryDSN string
	// r is a request performed when error occured
	var r *http.Request
	client, err := New(sentryDSN)
	if err != nil {
		log.Fatal(err)
	}
	trace := NewStacktrace(0, 2, nil)
	packet := NewPacket(raisedErr.Error(), NewException(raisedErr, trace), NewHttp(r))
	eventID, ch := client.Capture(packet, nil, nil)
	if err = <-ch; err != nil {
		log.Fatal(err)
	}
	message := fmt.Sprintf("Captured error with id %s: %q", eventID, raisedErr)
	log.Println(message)
}
