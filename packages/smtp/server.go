package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"dk.magnusjensen/mymail/lib/smtp"
	"dk.magnusjensen/mymail/lib/smtp/session"
)

type AuthService interface {
	AuthUser(username, password string) *User
}

type smtpServer struct {
	listener     net.Listener
	sendListener net.Listener
	cfg          *Config
	logger       *slog.Logger
	authSvc      AuthService

	pendingMails map[string]*smtp.MailTransaction
}

func NewSMTPServer(listener net.Listener, sendListener net.Listener, cfg *Config, logger *slog.Logger, authSvc AuthService) *smtpServer {
	return &smtpServer{
		listener:     listener,
		sendListener: sendListener,
		cfg:          cfg,
		logger:       logger,
		authSvc:      authSvc,
		pendingMails: map[string]*smtp.MailTransaction{},
	}
}

func (s *smtpServer) QueueMail(mail *smtp.MailTransaction) error {
	id := uuid.New().String()
	s.pendingMails[id] = mail
	// Write this to disk, as a temporary pending storage.
	jsonMail, err := json.MarshalIndent(mail, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to JSON marshall mail: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	if err := os.Mkdir(cwd+"/pending-mails", 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := os.WriteFile(cwd+"/pending-mails/"+id+".json", jsonMail, 0644); err != nil {
		return err
	}

	delete(s.pendingMails, id)
	return nil
}

func (s *smtpServer) Start() {
	go s.processPendingMail()

	// Listen for inbound connections
	go s.acceptInboundConnections()

	// Block and listen for send connections
	s.acceptSendConnections()
}

func (s *smtpServer) acceptInboundConnections() {
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

		serverSession := session.NewServerSession(s.logger, conn, s, session.InboundServerType, s.cfg.Hostname)
		if tlsCert != nil {
			serverSession.SetTLSCertificate(*tlsCert)
		}
		serverSession.Start()
	}
}

func (s *smtpServer) acceptSendConnections() {
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
		conn, err := s.sendListener.Accept()
		if err != nil {
			fmt.Printf("Failed to accept connection: %v\n", err)
			continue
		}

		serverSession := session.NewServerSession(s.logger, conn, s, session.SendServerType, s.cfg.Hostname)
		if tlsCert != nil {
			serverSession.SetTLSCertificate(*tlsCert)
		}
		serverSession.Start()
	}
}

func (s *smtpServer) processPendingMail() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		// For now we loop over each pending mail and attempt to send it, in the future
		// finding all to the same domain to avoid duplicate lookups.

		pendingMails, err := s.readLocalPendingMails()
		if err != nil {
			fmt.Printf("Failed reading pending mails from disk: %v", err)
			continue
		}

		for mailId, mail := range pendingMails {
			// TODO: Loop over all MX records and find the lowest preference
			hostName := mail.GetRemoteAddress()
			mxHost, err := smtp.GetPreferredMXHost(hostName)
			if err != nil {
				fmt.Println(err)
				continue
			}
			if err := s.relay(mail, mxHost); err != nil {
				fmt.Printf("Issue relaying mail: %v\n", err)
				continue
			}

			// Once we have successfully relayed the mail, delete it from the os.
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Printf("Failed to get working directory: %v", err)
				continue
			}

			pendingFile := filepath.Join(cwd, "pending-mails", mailId+".json")
			if err := os.Remove(pendingFile); err != nil {
				fmt.Printf("Failed to remove file from disk: %v", err)
				continue
			}
		}
	}
}

func (s *smtpServer) relay(mail *smtp.MailTransaction, remoteAddr string) error {
	//netConn, err := net.Dial("tcp", remoteAddr+":25")

	netConn, err := smtp.DialViaSocks5("127.0.0.1:1080", remoteAddr, 25)
	if err != nil {
		return err
	}
	defer netConn.Close()

	session := session.NewClientSession(s.logger, netConn, s.cfg.Hostname)
	return session.SendMail(mail)
}

func (s *smtpServer) readLocalPendingMails() (map[string]*smtp.MailTransaction, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	pendingDir := filepath.Join(cwd, "pending-mails")
	pendingMails := map[string]*smtp.MailTransaction{}

	entries, err := os.ReadDir(pendingDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return pendingMails, nil
		}
		return nil, fmt.Errorf("failed to read pending mails directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(pendingDir, entry.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read pending mail %q: %w", entry.Name(), err)
		}

		var mail smtp.MailTransaction
		if err := json.Unmarshal(data, &mail); err != nil {
			return nil, fmt.Errorf("failed to decode pending mail %q: %w", entry.Name(), err)
		}

		parts := strings.SplitN(entry.Name(), ".", 2)
		mailId := parts[0]
		pendingMails[mailId] = &mail
	}

	return pendingMails, nil
}

func (s *smtpServer) AuthenticateUser(username, password string) error {
	user := s.authSvc.AuthUser(username, password)
	if user == nil {
		return errors.New("Incorrect username or password")
	}

	return nil
}
