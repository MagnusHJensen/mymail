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
	"time"

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

	// Consecutive 5xx returns, once we hit 10 we drop the connection.
	// A successful reply resets this.
	consecutiveErrors int
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

	session := &ServerSession{
		conn:       NewConnection(netConn, serverLogger),
		logger:     logger,
		hostname:   hostname,
		state:      ServerSessionFresh,
		handler:    handler,
		serverType: serverType,
	}

	// Start with a 10 second deadline, the on any read/reply, we extend the deadline with 10 seconds.
	// TODO: update with per command timeouts as specified in
	// https://www.rfc-editor.org/info/rfc5321/#section-4.5.3.2
	session.ExtendDeadline(time.Second * 10)

	return session
}

func (s *ServerSession) ExtendDeadline(duration time.Duration) {
	s.conn.net.SetDeadline(time.Now().Add(duration))
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
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("encountered panic", "error", r)
		}
	}()
	defer s.Stop()
	if err := s.greet(); err != nil {
		s.shuttingDown()
		return
	}

	for {
		line, err := s.conn.Read(smtp.MaxCommandLineSize)
		if err != nil {
			fmt.Printf("Failed to read data: %v\n", err)
			s.shuttingDown()
			return
		}
		s.ExtendDeadline(time.Second * 10)

		if err := s.handleCommand(line); err != nil {
			if replyErr, ok := errors.AsType[*smtp.ReplyError](err); ok {
				if err := s.Reply(replyErr.GetCode(), replyErr.GetMsg()); err != nil {
					if innerReplyErr, ok := errors.AsType[*smtp.ReplyError](err); ok {
						s.conn.sendRaw(innerReplyErr.Error())
						replyErr = innerReplyErr
					} else {
						fmt.Printf("Failed to reply on a replyErr %v\n", err)
						continue
					}

				}

				if replyErr.GetCode() == smtp.CodeServiceNotAvailable || replyErr.GetCode() == smtp.CodeShuttingDown {
					// close the connection
					return
				}
				continue
			}

			// TODO: Wrap all errors with a reply (internal server error)

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
		return smtp.NewReplyError(smtp.CodeShuttingDown, fmt.Sprintf("%s Service closing down", s.hostname))
	case "MAIL":
		return s.handleMail(parts)
	case "RCPT":
		return s.handleRecipient(parts)
	case "DATA":
		return s.handleData()
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
	mailTransaction := &smtp.MailTransaction{
		From: from,
	}
	if s.serverType == InboundServerType {
		if mailTransaction.GetFromHostname() == s.hostname {
			// block mails from our own domain on the inbound port.
			return smtp.NewReplyError(smtp.CodeActionNotTaken, "Use the send address (:587) to send mails")
		}
	} else if s.serverType == SendServerType {
		if mailTransaction.GetFromHostname() != s.hostname {
			// block mails from other domains on our send port
			return smtp.NewReplyError(smtp.CodeActionNotTaken, "Use the MTA address (:25) to deliver mails")
		}
	}

	if s.activeTx != nil {
		return smtp.NewReplyError(smtp.CodeBadSequence, "A mail transaction is already in progress")
	}

	s.activeTx = mailTransaction
	return s.Reply(smtp.CodeOK, "")
}

func (s *ServerSession) handleRecipient(cmdParts []string) error {
	if err := s.requireAuth(); err != nil {
		return err
	}

	if len(cmdParts) < 2 {
		return smtp.NewReplyError(smtp.CodeSyntaxError, "MAIL command requires TO argument")
	}

	if s.activeTx == nil {
		return smtp.NewReplyError(smtp.CodeBadSequence, "RCPT requires starting a sequence via MAIL first")
	}

	to := strings.SplitN(cmdParts[1], ":", 2)[1]
	s.activeTx.To = &to

	if s.serverType == InboundServerType && s.activeTx.GetRemoteAddress() != s.hostname {
		// We will only handle inbound mails to our domain
		// TODO: 5.1.1
		s.activeTx = nil
		return smtp.NewReplyError(smtp.CodeActionNotTaken, "The email account you tried to reach does not exist")
	}

	return s.Reply(smtp.CodeOK, "")
}

func (s *ServerSession) handleData() error {
	if err := s.requireAuth(); err != nil {
		return err
	}
	// TODO: Error
	s.Reply(smtp.CodeStartData, "Start mail input; end with <CRLF>.<CRLF>")

	// Keep reading each incoming line (terminated by CRLF) until we hit a line with <CRLF>.<CRLF>
	lines := []string{}
	currentSize := 0
	for {
		dataLine, terminated, err := s.ReadDataLine(1000) // TODO: Pull into constant
		if err != nil {
			return err
		}
		if terminated {
			break
		}

		lines = append(lines, dataLine)
		currentSize += len(dataLine) + 2 // len(string) is a byte sequence = octets. +2 is from the never returned \r\n.

		if currentSize > 64*1000 { // TODO: Pull into const, and have a more lenient max size, today it's max 64KB
			s.activeTx = nil
			return smtp.NewReplyError(smtp.CodeStorageExceeded, "Too much mail data")
		}
	}

	s.activeTx.Data = lines
	pendingMail := s.activeTx
	s.activeTx = nil
	if err := s.handler.QueueMail(pendingMail); err != nil {
		return s.Reply(smtp.CodeTransactionFailed, "")
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
		return smtp.NewReplyError(smtp.CodeBadSequence, "Error: already authenticated")
	}

	if !s.conn.isTLSSessionActive {
		// TODO: 5.7.11
		return smtp.NewReplyError(smtp.CodeTLSRequired, "Encryption required for requested authentication mechanism")
	}

	if len(authParts) == 0 {
		return smtp.NewReplyError(smtp.CodeSyntaxError, "AUTH requires a mechanism")
	}
	mechanism := authParts[0]
	if !slices.Contains(SupportedAuthMechanisms, mechanism) {
		return smtp.NewReplyError(smtp.CodeActionNotTaken, "Client specified an unknown AUTH mechanism")
	}

	if len(authParts) < 2 {
		return smtp.NewReplyError(smtp.CodeSyntaxError, "PLAIN requires a base64 encoded data")
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
		s.logger.Error(err.Error())
		return smtp.NewReplyError(smtp.CodeAuthFailed, "Authentication failed")
	}

	// We successfully authenticated - we are now ready to accept mail
	s.authedUser = &authId
	s.state = ServerSessionReady
	return s.Reply(smtp.CodeAuthOK, "Ok")
}

func (s *ServerSession) ReplyMulti(code smtp.Code, lines []string) error {
	if smtp.IsFailureCode(code) {
		s.consecutiveErrors++
	} else {
		s.consecutiveErrors = 0
	}

	if s.consecutiveErrors >= 10 {
		return smtp.NewReplyError(smtp.CodeServiceNotAvailable, s.hostname)
	}

	s.ExtendDeadline(time.Second * 10)
	length := len(lines)
	lastLine := lines[length-1]
	for _, line := range lines[:length-1] {
		if err := s.conn.sendRaw(fmt.Sprintf("%d-%s", code, line)); err != nil {
			return err
		}
	}

	return s.Reply(code, lastLine)
}

func (s *ServerSession) Reply(code smtp.Code, args string) error {
	if smtp.IsFailureCode(code) {
		s.consecutiveErrors++
	} else {
		s.consecutiveErrors = 0
	}
	if s.consecutiveErrors >= 10 {
		return smtp.NewReplyError(smtp.CodeServiceNotAvailable, s.hostname)
	}

	reply := fmt.Sprintf("%d", code)
	if args != "" {
		reply += fmt.Sprintf(" %s", args)
	}
	s.ExtendDeadline(time.Second * 10)
	return s.conn.sendRaw(reply)
}

// requireAuth, checks for SendServerType auth was done.
func (s *ServerSession) requireAuth() error {
	if s.serverType == InboundServerType {
		return nil // no-op
	}

	if s.authedUser == nil {
		// TODO: EnhancedStatusCode = 5.7.0
		return smtp.NewReplyError(smtp.CodeAuthRequired, "Authentication required")
	}

	return nil
}

func (c *ServerSession) ReadDataLine(maxSize int) (line string, terminated bool, err error) {

	var buf []byte
	for {
		// For data lines we do this first as the DATA sequence terminator is <CRLF>.<CRLF>
		// After each byte read, peek the next two to see if its a CRLF sequence.
		nextBytes, err := c.conn.reader.Peek(2)
		if err != nil {
			// Whatever we do peek even if not both bytes, store it in the buffer
			if len(nextBytes) > 0 {
				buf = append(buf, nextBytes...)
			}
			return string(buf), false, err
		}
		if nextBytes[0] == '\r' && nextBytes[1] == '\n' {
			// no \r\n to trim anymore.
			// but advance the reader to avoid next read to start with these bytes
			c.conn.reader.Discard(2)

			// for data lines, we peek an additional 3 bytes, to check for ".<CRLF>" to indicate data sequence end
			nextBytes, err = c.conn.reader.Peek(3)
			if err != nil {
				if len(nextBytes) > 0 {
					buf = append(buf, nextBytes...)
				}
				return string(buf), false, err
			}
			if nextBytes[0] == '.' && nextBytes[1] == '\r' && nextBytes[2] == '\n' {
				c.conn.reader.Discard(3)
				return string(buf), true, nil
			}

			c.logger.Debug(fmt.Sprintf("received %s", string(buf)))
			return string(buf), false, nil
		}

		b, err := c.conn.reader.ReadByte()
		if err != nil {
			return string(buf), false, err
		}
		buf = append(buf, b)
		if len(buf) > maxSize-2 { // -2 to accomodate for the <CRLF>
			return "", false, fmt.Errorf("line exceeds max length of %d bytes", maxSize)
		}
	}
}
