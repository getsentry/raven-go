package main

import (
	"github.com/cupcake/raven-go"
	"log"
	"os"
)

func main() {
	_, err := raven.NewClient(os.Args[1], nil)
	if err != nil {
		log.Fatalln(err)
	}

	log.Println("raven: that's a fine DSN you have there")
}
