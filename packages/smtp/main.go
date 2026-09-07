package main

import (
	"fmt"
	"net"
)

func main() {
	listener, err := net.Listen("tcp", ":2525")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	fmt.Printf("SMTP Server listening on port 2525\n")

	server := NewSMTPServer(listener)
	server.Start()
}
