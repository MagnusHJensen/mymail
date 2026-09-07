package smtp

import (
	"fmt"
	"net"
	"strings"
)

type ServerSession struct {
	conn *Connection

	state    ServerSessionState
	activeTx *MailTransaction
	handler  ServerSessionHandler
}

type ServerSessionHandler interface {
	QueueMail(mail *MailTransaction) error
}

type ServerSessionState int

const (
	ServerSessionFresh ServerSessionState = iota
	ServerSessionReady
)

func NewServerSession(netConn net.Conn, handler ServerSessionHandler) *ServerSession {
	return &ServerSession{
		conn:    NewConnection(netConn, ServerSide),
		state:   ServerSessionFresh,
		handler: handler,
	}
}

func (s *ServerSession) Start() {
	go s.serve()
}

func (s *ServerSession) Stop() {
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
	switch Command(cmd) {
	case ExtendedHelloCmd, HelloCmd:
		// After hello, move to ready
		s.state = ServerSessionReady
		return s.conn.Send(CodeOK)
	case "QUIT":
		return s.conn.SendWithArgs(CodeShuttingDown, "localhost Service closing transmission channel")
	case "MAIL":
		if len(parts) < 2 {
			return s.conn.SendWithArgs(CodeSyntaxError, "MAIL command requires FROM argument")
		}

		/* if !conn.canReceiveMail() {
			conn.sendCommand("503 Must identify first with EHLO or HELO")
			return nil, false
		} */

		from := strings.SplitN(parts[1], ":", 2)[1]
		s.activeTx = &MailTransaction{
			From: from,
		}
		return s.conn.Send(CodeOK)
	case "RCPT":
		if len(parts) < 2 {
			return s.conn.SendWithArgs(CodeSyntaxError, "MAIL command requires TO argument")
		}

		if s.activeTx == nil {
			return s.conn.SendWithArgs(CodeBadSequence, "RCPT requires starting a sequence via MAIL first")
		}

		to := strings.SplitN(parts[1], ":", 2)[1]
		s.activeTx.To = &to

		return s.conn.Send(CodeOK)
	case "DATA":
		// TODO: Error
		s.conn.SendWithArgs(CodeStartData, "Start mail input; end with <CRLF>.<CRLF>")

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
			return s.conn.Send(CodeTransactionFailed)
		}
		return s.conn.Send(CodeOK)
	default:
		if err := s.conn.Send(CodeNotImplemented); err != nil {
			return err
		}
		return fmt.Errorf("Command %q is not supported", cmd)
	}
}

func (s *ServerSession) greet() error {
	return s.conn.SendWithArgs(CodeReady, "localhost")
}

func (s *ServerSession) shuttingDown() {
	if err := s.conn.Send(CodeShuttingDown); err != nil {
		fmt.Printf("Failed to send shutting down: %v\n", err)
	}
}
