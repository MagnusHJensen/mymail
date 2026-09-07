package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"dk.magnusjensen/mymail/lib/smtp"
)

type smtpServer struct {
	listener net.Listener
	cfg      *Config

	pendingMails map[string]*smtp.MailTransaction
}

func NewSMTPServer(listener net.Listener, cfg *Config) *smtpServer {
	return &smtpServer{
		listener:     listener,
		cfg:          cfg,
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

		serverSession := smtp.NewServerSession(conn, s, s.cfg.Hostname)
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

	netConn, err := dialViaSocks5("127.0.0.1:1080", remoteAddr, 25)
	if err != nil {
		return err
	}
	defer netConn.Close()

	session := smtp.NewClientSession(netConn, s.cfg.Hostname)
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

func dialViaSocks5(proxyAddr, targetHost string, targetPort int) (net.Conn, error) {
	conn, err := net.Dial("tcp", proxyAddr) // "127.0.0.1:1080"
	if err != nil {
		return nil, err
	}

	// greeting: version 5, 1 auth method, "no auth"
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		conn.Close()
		return nil, err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil || resp[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 handshake failed")
	}

	// Resolve IPv4 locally so we control the address family — the SOCKS5
	// server's own resolver prefers AAAA, which Gmail rejects without PTR/SPF.
	ipAddr, err := net.ResolveIPAddr("ip4", targetHost)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("no IPv4 address for %s: %w", targetHost, err)
	}
	ip4 := ipAddr.IP.To4()

	req := []byte{0x05, 0x01, 0x00, 0x01} // CONNECT, IPv4 addr type
	req = append(req, ip4...)
	req = append(req, byte(targetPort>>8), byte(targetPort))
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}

	// reply: ver, rep, rsv, atyp, then variable bind addr+port
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil || head[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 connect failed: rep=%d", head[1])
	}
	switch head[3] {
	case 0x01:
		io.CopyN(io.Discard, conn, 4+2) // IPv4 + port
	case 0x04:
		io.CopyN(io.Discard, conn, 16+2) // IPv6 + port
	}

	return conn, nil
}
