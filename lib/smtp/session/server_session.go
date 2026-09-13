package session

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strings"

	"dk.magnusjensen/mymail/lib/smtp"
	"dk.magnusjensen/mymail/lib/smtp/sasl"
)

type ServerSession struct {
	conn       *Connection
	logger     *slog.Logger
	hostname   string
	serverType ServerType
	handler    ServerSessionHandler

	// tls
	tlsCertificate tls.Certificate

	state      ServerSessionState
	activeTx   *smtp.MailTransaction
	authedUser *string // TODO: Better types etc. for now it's the authID if it passed authentication
}

type ServerSessionHandler interface {
	QueueMail(mail *smtp.MailTransaction) error
	AuthenticateUser(username, password string) error
}

type ServerSessionState int

const (
	ServerSessionFresh ServerSessionState = iota
	ServerSessionReady
)

type ServerType int

const (
	InboundServerType ServerType = iota
	SendServerType
)

var SupportedAuthMechanisms = []string{sasl.PlainMechanism}

func NewServerSession(logger *slog.Logger, netConn net.Conn, handler ServerSessionHandler, serverType ServerType, hostname string) *ServerSession {
	serverLogger := logger.With("as", "server")
	switch serverType {
	case InboundServerType:
		serverLogger = serverLogger.With("type", "inbound")
	case SendServerType:
		serverLogger = serverLogger.With("type", "send")
	}
	return &ServerSession{
		conn:       NewConnection(netConn, serverLogger),
		logger:     logger,
		hostname:   hostname,
		state:      ServerSessionFresh,
		handler:    handler,
		serverType: serverType,
	}
}

func (s *ServerSession) SetTLSCertificate(cert tls.Certificate) {
	s.tlsCertificate = cert
}

func (s *ServerSession) Start() {
	s.logger.Info("Started server session")
	go s.serve()
}

func (s *ServerSession) Stop() {
	s.logger.Info("Stopping server session")
	s.conn.net.Close()
}

func (s *ServerSession) serve() {
	defer s.Stop()
	if err := s.greet(); err != nil {
		s.shuttingDown()
		return
	}

	for {
		line, err := s.conn.Read()
		if err != nil {
			fmt.Printf("Failed to read data: %v\n", err)
			s.shuttingDown()
			return
		}

		if err := s.handleCommand(line); err != nil {
			// log, never drop the connection.
			// TODO: Figure out when to drop connection to avoid writing/reading on a stale connection
			fmt.Printf("Failed to handle command: %v\n", err)
		}
	}

}

func (s *ServerSession) handleCommand(line string) error {
	parts := strings.SplitN(line, " ", 3)

	cmd := strings.ToUpper(parts[0])
	switch smtp.Command(cmd) {
	case smtp.ExtendedHelloCmd:
		return s.handleExtendedHello()
	case "QUIT":
		return s.Reply(smtp.CodeShuttingDown, "localhost Service closing transmission channel")
	case "MAIL":
		return s.handleMail(parts)
	case "RCPT":
		return s.handleRecipient(parts)
	case "DATA":
		s.requireAuth()
		// TODO: Error
		s.Reply(smtp.CodeStartData, "Start mail input; end with <CRLF>.<CRLF>")

		// Keep reading each incoming line (terminated by CRLF) until we hit a line with <CRLF>.<CRLF>
		lines := []string{}
		for {
			dataLine, err := s.conn.Read()
			if err != nil {
				return err
			}
			// TODO: We trim the right CRLF, so we match on dot. If a client sends a "." on a line by itself, it could terminate.
			if dataLine == "." {
				break
			}

			lines = append(lines, dataLine)
		}

		s.activeTx.Data = lines
		pendingMail := s.activeTx
		s.activeTx = nil
		if err := s.handler.QueueMail(pendingMail); err != nil {
			return s.Reply(smtp.CodeTransactionFailed, "")
		}
		return s.Reply(smtp.CodeOK, "")
	case smtp.StartTLSCmd:
		if err := s.Reply(smtp.CodeReady, "Ready for TLS"); err != nil {
			return err
		}
		s.UpgradeToTLS()
		return nil
	case smtp.AuthCmd:
		return s.HandleAuth(parts[1:])
	default:
		if err := s.Reply(smtp.CodeNotImplemented, ""); err != nil {
			return err
		}
		return fmt.Errorf("Command %q is not supported", cmd)
	}
}

func (s *ServerSession) greet() error {
	return s.Reply(smtp.CodeReady, s.hostname)
}

func (s *ServerSession) handleExtendedHello() error {
	replies := []string{s.hostname}
	if !s.conn.isTLSSessionActive {
		// report STARTTLS if not already active
		replies = append(replies, "STARTTLS")
	}
	if s.serverType == InboundServerType {
		// for inbound we can start accepting mail from here
		s.state = ServerSessionReady
		return s.ReplyMulti(smtp.CodeOK, replies)
	}

	if s.conn.isTLSSessionActive && s.state != ServerSessionReady {
		// report AUTH for Send server type when TLS connected and not already authed.
		replies = append(replies, fmt.Sprintf("%s %s", "AUTH", sasl.PlainMechanism))
	}
	return s.ReplyMulti(smtp.CodeOK, replies)
}

func (s *ServerSession) handleMail(cmdParts []string) error {
	if err := s.requireAuth(); err != nil {
		return err
	}

	if len(cmdParts) < 2 {
		return s.Reply(smtp.CodeSyntaxError, "MAIL command requires FROM argument")
	}

	/* if !s.canReceiveMail() {
		conn.sendCommand("503 Must identify first with EHLO or HELO")
		return nil, false
	} */

	from := strings.SplitN(cmdParts[1], ":", 2)[1]
	s.activeTx = &smtp.MailTransaction{
		From: from,
	}
	if s.serverType == InboundServerType {
		if s.activeTx.GetFromHostname() == s.hostname {
			// block mails from our own domain on the inbound port.
			return s.Reply(smtp.CodeActionNotTaken, "Use the send address (:587) to send mails")
		}
	} else if s.serverType == SendServerType {
		if s.activeTx.GetFromHostname() != s.hostname {
			// block mails from other domains on our send port
			return s.Reply(smtp.CodeActionNotTaken, "Use the MTA address (:25) to deliver mails")
		}
	}
	/*  */

	return s.Reply(smtp.CodeOK, "")
}

func (s *ServerSession) handleRecipient(cmdParts []string) error {
	if err := s.requireAuth(); err != nil {
		return err
	}

	if len(cmdParts) < 2 {
		return s.Reply(smtp.CodeSyntaxError, "MAIL command requires TO argument")
	}

	if s.activeTx == nil {
		return s.Reply(smtp.CodeBadSequence, "RCPT requires starting a sequence via MAIL first")
	}

	to := strings.SplitN(cmdParts[1], ":", 2)[1]
	s.activeTx.To = &to

	if s.serverType == InboundServerType && s.activeTx.GetRemoteAddress() != s.hostname {
		// We will only handle inbound mails to our domain
		// TODO: 5.1.1
		return s.Reply(smtp.CodeActionNotTaken, "The email account you tried to reach does not exist.")
	}

	return s.Reply(smtp.CodeOK, "")
}

func (s *ServerSession) shuttingDown() {
	if err := s.Reply(smtp.CodeShuttingDown, s.hostname); err != nil {
		fmt.Printf("Failed to send shutting down: %v\n", err)
	}
}

func (c *ServerSession) UpgradeToTLS() {
	// Wrap the current connection with a TLS server
	tlsServer := tls.Server(c.conn.net, &tls.Config{
		Certificates: []tls.Certificate{
			c.tlsCertificate,
		},
	})
	if err := tlsServer.Handshake(); err != nil {
		panic(err)
	}
	c.conn.net = tlsServer
	c.conn.reader = bufio.NewReader(c.conn.net)
	c.conn.isTLSSessionActive = true
}

func (s *ServerSession) HandleAuth(authParts []string) error {
	if s.authedUser != nil {
		// TODO: Enhanced status codes 5.5.1
		return s.Reply(smtp.CodeBadSequence, "Error: already authenticated")
	}

	if len(authParts) == 0 {
		return errors.New("AUTH requires at least the mechanism")
	}
	mechanism := authParts[0]
	if !slices.Contains(SupportedAuthMechanisms, mechanism) {
		return errors.New("Client specified an unknown mechanism")
	}

	if len(authParts) < 2 {
		return errors.New("PLAIN requires base64 encoded data")
	}
	// TODO: We only support plain
	authEncoded := authParts[1]
	authText, err := base64.StdEncoding.DecodeString(authEncoded)
	if err != nil {
		return err
	}

	// authText is now a SASL PLAIN string
	_, authId, password, err := sasl.ParsePlainAuthMessage(string(authText))
	if err != nil {
		return err
	}

	if err := s.handler.AuthenticateUser(authId, password); err != nil {
		return s.Reply(smtp.CodeAuthFailed, "Authentication failed")
	}

	// We successfully authenticated - we are now ready to accept mail
	s.authedUser = &authId
	s.state = ServerSessionReady
	return s.Reply(smtp.CodeAuthOK, "Ok")
}

func (c *ServerSession) ReplyMulti(code smtp.Code, lines []string) error {
	length := len(lines)
	lastLine := lines[length-1]
	for _, line := range lines[:length-1] {
		if err := c.conn.sendRaw(fmt.Sprintf("%d-%s", code, line)); err != nil {
			return err
		}
	}

	return c.Reply(code, lastLine)
}

func (c *ServerSession) Reply(code smtp.Code, args string) error {
	reply := fmt.Sprintf("%d", code)
	if args != "" {
		reply += fmt.Sprintf(" %s", args)
	}
	return c.conn.sendRaw(reply)
}

// requireAuth, checks for SendServerType auth was done.
func (s *ServerSession) requireAuth() error {
	if s.serverType == InboundServerType {
		return nil // no-op
	}

	if s.authedUser == nil {
		// TODO: EnhancedStatusCode = 5.7.1
		return s.Reply(smtp.CodeActionNotTaken, "Authentication required")
	}

	return nil
}
