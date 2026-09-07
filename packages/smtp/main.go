package main

import (
	"fmt"
	"net"
)

func main() {
	cfg := LoadConfig()

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	fmt.Printf("SMTP Server listening on %s\n", cfg.ListenAddr)

	server := NewSMTPServer(listener, cfg)
	server.Start()
}
