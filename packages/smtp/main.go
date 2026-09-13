package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"
)

func main() {
	cfg := LoadConfig()

	logHandlerOptions := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	if cfg.Debug {
		logHandlerOptions.Level = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, logHandlerOptions))

	authSvc := NewAuthService(logger, "users.json")

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	fmt.Printf("SMTP Server listening on %s\n", cfg.ListenAddr)

	sendListener, err := net.Listen("tcp", cfg.SendListenAddr)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	fmt.Printf("SMTP Server listening for sends on %s\n", cfg.SendListenAddr)

	server := NewSMTPServer(listener, sendListener, cfg, logger, authSvc)
	server.Start()
}
