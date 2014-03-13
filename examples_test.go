package raven

import (
	"fmt"
	"log"
	"os"
)

func Example() {
	// ... i.e. raisedErr is incoming error
	client, err := raven.NewClient(sentryDSN, map[string]string{})
	if err != nil {
		log.Fatal(err)
	}
	packet := raven.NewPacket(raisedErr.Error(), raven.NewException(raisedErr, trace()), raven.NewHttp(r))
	eventID, ch := client.Capture(packet, nil)
	if err = <-ch; err != nil {
		log.Fatal(err)
	}
	message := fmt.Sprintf("Error event with id \"%s\" - %s", eventID, raisedErr.Error())
	log.Println(message)
}

func ExamplePacket() {
	// os.Args[1] is sentry DSN string
	client, err := raven.NewClient(os.Args[1], map[string]string{"foo": "bar"})
	if err != nil {
		log.Fatal(err)
	}
	req, _ := http.NewRequest("GET", "http://example.com/foo?bar=true", nil)
	req.RemoteAddr = "127.0.0.1:80"
	req.Header = http.Header{"Content-Type": {"text/html"}, "Content-Length": {"42"}}
	packet := &raven.Packet{Message: "Test report", Interfaces: []raven.Interface{raven.NewException(errors.New("example"), trace()), raven.NewHttp(req)}}
	_, ch := client.Capture(packet, nil)
	if err = <-ch; err != nil {
		log.Fatal(err)
	}
	log.Print("sent packet successfully")
}
