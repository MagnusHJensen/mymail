package session

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"dk.magnusjensen/mymail/lib/smtp"
)

type ClientSession struct {
	conn     *Connection
	hostname string
}

func NewClientSession(netConn net.Conn, hostname string) *ClientSession {
	return &ClientSession{
		conn:     NewConnection(netConn, ClientSide),
		hostname: hostname,
	}
}

func (s *ClientSession) SendMail(mail *smtp.MailTransaction) error {
	if err := s.ReadGreeting(); err != nil {
		return err
	}

	s.ExtendedHello()

	/* if err := s.SendCommand(HelloCmd, s.hostname); err != nil {
		return err
	}
	if code, _, err := s.ReadReply(); err != nil {
		return err
	} else if code != int(CodeOK) {
		return fmt.Errorf("Got non OK code for %s: %d", ExtendedHelloCmd, code)
	} */

	if err := s.SendCommand(smtp.MailCmd, fmt.Sprintf("FROM:%s", mail.From)); err != nil {
		return err
	}
	if code, _, err := s.ReadReply(); err != nil {
		return err
	} else if code != int(smtp.CodeOK) {
		return fmt.Errorf("Got non OK code for %s: %d", smtp.MailCmd, code)
	}

	if err := s.SendCommand(smtp.RecipientCmd, fmt.Sprintf("TO:%s", *mail.To)); err != nil {
		return err
	}
	if code, _, err := s.ReadReply(); err != nil {
		return err
	} else if code != int(smtp.CodeOK) {
		return fmt.Errorf("Got non OK code for %s: %d", smtp.RecipientCmd, code)
	}

	if err := s.SendCommand(smtp.DataCmd, ""); err != nil {
		return err
	}
	if _, err := s.conn.Read(); err != nil {
		return err
	}

	// send each line in data one-by-one, finish with <CRLF>.<CRLF>

	for _, line := range mail.Data {
		if err := s.conn.sendRaw(line); err != nil {
			return err
		}
	}

	if err := s.conn.sendRaw("."); err != nil {
		return err
	}
	code, lines, err := s.ReadReply()
	if err != nil {
		return err
	}
	if code != int(smtp.CodeOK) {
		return fmt.Errorf("Got non OK status code: %d\n%v\n", code, lines)
	}

	s.Quit()
	return nil
}

func (s *ClientSession) ReadGreeting() error {
	if code, lines, err := s.ReadReply(); err != nil {
		return err
	} else if code != int(smtp.CodeReady) {
		return fmt.Errorf("Did not receive greeting code on greeting: %d\n - %v", code, lines)
	}
	return nil
}

func (s *ClientSession) ExtendedHello() error {
	if err := s.SendCommand(smtp.ExtendedHelloCmd, s.hostname); err != nil {
		return err
	}
	code, lines, err := s.ReadReply()
	if err != nil {
		return err
	} else if code != int(smtp.CodeOK) {
		return fmt.Errorf("Did not receive OK on %s: %d\n - %v", smtp.ExtendedHelloCmd, code, lines)
	}

	return s.handleCapabilities(lines)
}

func (s *ClientSession) handleCapabilities(lines []string) error {
	for _, line := range lines {
		if strings.Contains(line, "STARTTLS") {
			return s.StartTLS()
		}
	}

	return nil
}

func (s *ClientSession) StartTLS() error {
	fmt.Printf("Starting TLS connection\n")
	if s.conn.isTLSSessionActive {
		return errors.New("TLS connection is already active")
	}

	if err := s.SendCommand(smtp.StartTLSCmd, ""); err != nil {
		return err
	}
	if code, lines, err := s.ReadReply(); err != nil {
		return err
	} else if code != int(smtp.CodeReady) {
		return fmt.Errorf("Did not receive READY from %s: %d\n - %v", smtp.StartTLSCmd, code, lines)
	}

	// upgrade the connection to TLS
	s.conn.UpgradeToTLS()
	// re-trigger a new HELO OR EHLO
	return s.ExtendedHello()
}

func (s *ClientSession) Quit() {
	s.SendCommand(smtp.QuitCmd, "")
	s.conn.net.Close()
}

func (c *ClientSession) ReadReply() (code int, lines []string, err error) {
	for {
		line, err := c.conn.Read()
		if err != nil {
			return 0, nil, err
		}
		if err := c.validateReplyLine(line); err != nil {
			return 0, nil, err
		}
		parsedCode, err := strconv.ParseInt(line[:3], 10, 32)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to parse status code: %w", err)
		}
		code = int(parsedCode)
		lines = append(lines, line)
		if len(line) < 4 || line[3] != '-' {
			// terminate
			return code, lines, nil
		}
	}
}

// validateReplyLine validates an SMTP reply line based on the RFC ABNF
// https://www.rfc-editor.org/info/rfc5321/#section-4.2
// ABNF:
// Reply-line     = *( Reply-code "-" [ textstring ] CRLF )
//
//	Reply-code [ SP textstring ] CRLF
//
// Reply-code     = %x32-35 %x30-35 %x30-39
func (c *ClientSession) validateReplyLine(line string) error {
	if len(line) < 5 {
		return errors.New("reply line cannot be less than 5 octets - 3 digits and <CRLF>")
	}

	/* Commented out since it's stripped in the reader and I haven't decided yet.
	if !strings.HasSuffix(line, "\r\n") {
		return errors.New("all reply lines must be terminated by <CRLF>")
	}
	*/

	if line[0] < '2' || line[0] > '5' {
		return fmt.Errorf("first reply code digit must be between 2 and 5 inclusive")
	}
	if line[1] < '0' || line[1] > '5' {
		return fmt.Errorf("second reply code digit must be between 0 and 5 inclusive")
	}
	if line[1] < '0' || line[2] > '9' {
		return errors.New("third reply code digit must be between 0 and 9 inclusive")
	}

	if len(line) > 5 {
		// We know it's not three digits and <CRLF>
		if line[3] != ' ' && line[3] != '-' {
			return errors.New("reply code must be separated by space or '-'")
		}
	}

	return nil
}

func (c *ClientSession) SendCommand(cmd smtp.Command, args string) error {
	return c.conn.sendRaw(fmt.Sprintf("%s %s", string(cmd), args))
}
