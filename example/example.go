package main

import (
	"github.com/cupcake/raven-go"
	"log"
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
	packet := &raven.Packet{Message: "Test report", Interfaces: []raven.Interface{trace()}}
	err = client.Send(packet)
	if err != nil {
		log.Fatal(err)
	}
	log.Print("sent packet successfully")
}
