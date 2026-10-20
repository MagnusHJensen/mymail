package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"dk.magnusjensen/mymail/lib/imap/session"
	"dk.magnusjensen/mymail/packages/shared"
)

type imapServer struct {
	listener net.Listener
	cfg      *Config
	logger   *slog.Logger

	authSvc authService
}

type authService interface {
	AuthUser(username, password string) *shared.User
}

func NewIMAPServer(listener net.Listener, cfg *Config, logger *slog.Logger, authSvc authService) *imapServer {
	return &imapServer{
		listener: listener,
		cfg:      cfg,
		logger:   logger,

		authSvc: authSvc,
	}
}

func (s *imapServer) Start() {
	// Listen for inbound connections
	s.acceptInboundConnections()
}

func (s *imapServer) acceptInboundConnections() {
	var tlsCert *tls.Certificate
	if s.cfg.TLSCertFile != "" {
		cert, err := tls.LoadX509KeyPair(s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
		if err != nil {
			fmt.Printf("Failed to load TLS cert: %v\n", err)
			return
		}
		tlsCert = &cert
	}
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			fmt.Printf("Failed to accept connection: %v\n", err)
			continue
		}

		session := session.NewIMAPServerSession(s.logger, conn, s)
		if tlsCert != nil {
			session.SetTLSCertificate(*tlsCert)
		}
		session.Start()
	}
}

func (s *imapServer) AuthenticateUser(username, password string) error {
	user := s.authSvc.AuthUser(username, password)
	if user == nil {
		return errors.New("user not found")
	}

	return nil
}
