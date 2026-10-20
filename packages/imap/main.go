package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"

	"dk.magnusjensen/mymail/packages/shared"
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

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	logger.Info(fmt.Sprintf("IMAP listening on %s", cfg.ListenAddr))

	authSvc := shared.NewAuthService(logger, "users.json")

	server := NewIMAPServer(listener, cfg, logger, authSvc)
	server.Start()
}
