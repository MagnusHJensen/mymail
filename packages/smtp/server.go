package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"dk.magnusjensen/mymail/lib/smtp"
)

type smtpServer struct {
	listener net.Listener

	pendingMails []*smtp.MailTransaction
}

func NewSMTPServer(listener net.Listener) *smtpServer {
	return &smtpServer{
		listener:     listener,
		pendingMails: []*smtp.MailTransaction{},
	}
}

func (s *smtpServer) QueueMail(mail *smtp.MailTransaction) error {
	s.pendingMails = append(s.pendingMails, mail)

	// Write this to disk, as a temporary pending storage.
	jsonMail, err := json.MarshalIndent(mail, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to JSON marshall mail: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	currentTimeDate := time.Now()

	if err := os.Mkdir(cwd+"/pending-mails", 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := os.WriteFile(cwd+"/pending-mails/"+currentTimeDate.Format(time.RFC3339)+".json", jsonMail, 0644); err != nil {
		return err
	}

	return nil
}

func (s *smtpServer) Start() {
	if err := s.readLocalPendingMails(); err != nil {
		fmt.Printf("Failed to read local pending mails: %v\n", err)
	}

	go s.processPendingMail()

	// Block the server with listening for connections
	s.acceptConnections()
}

func (s *smtpServer) acceptConnections() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			fmt.Printf("Failed to accept connection: %v\n", err)
			continue
		}

		serverSession := smtp.NewServerSession(conn, s)
		serverSession.Start()
	}
}

func (s *smtpServer) processPendingMail() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		// For now we loop over each pending mail and attempt to send it, in the future
		// finding all to the same domain to avoid duplicate lookups.
		for _, mail := range s.pendingMails {

			// TODO: Loop over all MX records and find the lowest preference
			hostName := mail.GetRemoteAddress()
			mxRecords, err := net.LookupMX(hostName)
			if err != nil {
				fmt.Printf("Issue processing mail: %v\n", err)
				continue
			}

			if len(mxRecords) < 1 {
				fmt.Printf("NO error, but no MX records found.\n")
				continue
			}

			record := mxRecords[0]
			if err := s.relay(mail, record.Host); err != nil {
				fmt.Printf("Issue relaying mail: %v\n", err)
				continue
			}
		}
	}
}

func (s *smtpServer) relay(mail *smtp.MailTransaction, remoteAddr string) error {
	netConn, err := net.Dial("tcp", remoteAddr+":25")
	if err != nil {
		return err
	}
	defer netConn.Close()

	session := smtp.NewClientSession(netConn)
	return session.SendMail(mail)
}

// Small developer/test function that reads any pending-mails on startup.
func (s *smtpServer) readLocalPendingMails() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	pendingDir := filepath.Join(cwd, "pending-mails")

	entries, err := os.ReadDir(pendingDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to read pending mails directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(pendingDir, entry.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read pending mail %q: %w", entry.Name(), err)
		}

		var mail smtp.MailTransaction
		if err := json.Unmarshal(data, &mail); err != nil {
			return fmt.Errorf("failed to decode pending mail %q: %w", entry.Name(), err)
		}

		s.pendingMails = append(s.pendingMails, &mail)

		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to remove pending mail %q: %w", entry.Name(), err)
		}
	}

	return nil
}
