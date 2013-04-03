package main

import (
	"github.com/cupcake/raven-go"
	"log"
	"net/http"
	"os"
)

func trace() *raven.Stacktrace {
	return raven.NewStacktrace(0, 2, nil)
}

func main() {
	client, err := raven.NewClient(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	httpReq, _ := http.NewRequest("GET", "http://example.com/foo?bar=true", nil)
	httpReq.RemoteAddr = "127.0.0.1:80"
	httpReq.Header = http.Header{"Content-Type": {"text/html"}, "Content-Length": {"42"}}
	packet := &raven.Packet{Message: "Test report", Interfaces: []raven.Interface{trace(), raven.NewHttp(httpReq)}}
	err = client.Send(packet)
	if err != nil {
		log.Fatal(err)
	}
	log.Print("sent packet successfully")
}
