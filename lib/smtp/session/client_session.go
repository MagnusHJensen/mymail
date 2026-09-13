package session

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"

	"dk.magnusjensen/mymail/lib/smtp"
	"dk.magnusjensen/mymail/lib/smtp/sasl"
)

type ClientSessionState int

const (
	ClientSessionFresh ClientSessionState = iota
	ClientSessionAuthed
	ClientSessionReady
)

type ClientSession struct {
	conn   *Connection
	logger *slog.Logger

	hostname string
	state    ClientSessionState

	authenticationId string
	password         string
}

type ClientSessionOption func(s *ClientSession)

// Implemented as an option, since our own relay client does not need it, but tooling does
func WithPlainAuth(authenticationId, password string) ClientSessionOption {
	return func(s *ClientSession) {
		s.authenticationId = authenticationId
		s.password = password
	}
}

func NewClientSession(logger *slog.Logger, netConn net.Conn, hostname string, opts ...ClientSessionOption) *ClientSession {
	clientLogger := logger.With("as", "client")
	session := &ClientSession{
		conn:     NewConnection(netConn, clientLogger),
		logger:   clientLogger,
		hostname: hostname,
		state:    ClientSessionFresh,
	}

	for _, opt := range opts {
		opt(session)
	}

	return session
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

	if err := s.Mail(mail); err != nil {
		return err
	}

	if err := s.Recipient(mail); err != nil {
		return err
	}

	if err := s.sendCommand(smtp.DataCmd, ""); err != nil {
		return err
	}
	if _, _, err := s.ReadReply(); err != nil {
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
	if err := s.sendCommand(smtp.ExtendedHelloCmd, s.hostname); err != nil {
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
		// Always check for TLS first before others.
		if strings.Contains(line, "STARTTLS") && !s.conn.isTLSSessionActive {
			return s.StartTLS()
		}

		if strings.Contains(line, "AUTH") && s.state == ClientSessionFresh {
			return s.InitAuth(line)
		}
	}

	s.logger.Info("ready to send")
	// we have handled all capabilities, we are now ready
	s.state = ClientSessionReady
	return nil
}

// StartTLS handles the STARTTLS[1] SMTP extension
//
// [1] https://datatracker.ietf.org/doc/html/rfc3207
func (s *ClientSession) StartTLS() error {
	s.logger.Info("Starting TLS connection")
	if s.conn.isTLSSessionActive {
		return errors.New("TLS connection is already active")
	}

	if err := s.sendCommand(smtp.StartTLSCmd, ""); err != nil {
		return err
	}
	if code, lines, err := s.ReadReply(); err != nil {
		return err
	} else if code != int(smtp.CodeReady) {
		return fmt.Errorf("Did not receive READY from %s: %d\n - %v", smtp.StartTLSCmd, code, lines)
	}

	// upgrade the connection to TLS
	s.UpgradeToTLS()
	// re-trigger a new HELO OR EHLO
	return s.ExtendedHello()
}

// InitAuth handles the AUTH[1] SMTP extension
//
// [1] https://datatracker.ietf.org/doc/html/rfc4954
func (s *ClientSession) InitAuth(authLine string) error {
	if !s.conn.isTLSSessionActive {
		return errors.New("AUTH requires an active TLS connection")
	}

	// Find the first appropriate auth method, should be selected on the most secure.
	// TODO: Initial implementation supports PLAIN only.
	var splitLine []string
	if authLine[3] == '-' {
		splitLine = strings.SplitN(authLine, "-", 2)
	} else {
		// split by space as per the ABNF
		splitLine = strings.SplitN(authLine, " ", 2)
	}

	// Now get a list of all available SASL mechanisms the serve supports
	mechanisms := strings.Split(splitLine[1], " ")
	// AUTH is the first here, so everything after that
	var selectedMechanism string
	for _, mechanism := range mechanisms {
		if mechanism == sasl.PlainMechanism {
			selectedMechanism = mechanism
		}
	}

	if selectedMechanism == "" {
		return errors.New("did not find a supported SASL mechanism")
	}

	if s.authenticationId == "" || s.password == "" {
		return errors.New("client was not initialized with WithPlainAuth")
	}

	// TODO: For now we only handle plain
	authMsg := sasl.PlainAuthMessage(s.authenticationId, s.password)
	encoded := base64.StdEncoding.EncodeToString([]byte(authMsg))

	if err := s.sendCommand(smtp.AuthCmd, fmt.Sprintf("%s %s", selectedMechanism, encoded)); err != nil {
		return err
	}
	if code, lines, err := s.ReadReply(); err != nil {
		return err
	} else if code != 235 { // TODO: Document as const code
		return fmt.Errorf("received non 235 code for %s: %d\n - %v", smtp.AuthCmd, code, lines)
	}

	s.state = ClientSessionAuthed
	return s.ExtendedHello()
}

func (s *ClientSession) Mail(mail *smtp.MailTransaction) error {
	if err := s.sendCommand(smtp.MailCmd, fmt.Sprintf("FROM:<%s>", *mail.To)); err != nil {
		return err
	}
	if code, _, err := s.ReadReply(); err != nil {
		return err
	} else if code != int(smtp.CodeOK) {
		return fmt.Errorf("Got non OK code for %s: %d", smtp.MailCmd, code)
	}

	return nil
}

func (s *ClientSession) Recipient(mail *smtp.MailTransaction) error {
	if err := s.sendCommand(smtp.RecipientCmd, fmt.Sprintf("TO:<%s>", *mail.To)); err != nil {
		return err
	}
	if code, _, err := s.ReadReply(); err != nil {
		return err
	} else if code != int(smtp.CodeOK) {
		return fmt.Errorf("Got non OK code for %s: %d", smtp.RecipientCmd, code)
	}
	return nil
}

func (s *ClientSession) Quit() {
	s.sendCommand(smtp.QuitCmd, "")
	s.conn.net.Close()
}

// Reply line has a max size of 512 octets
// https://www.rfc-editor.org/info/rfc5321/#section-4.5.3.1.5
const MaxReplyLineSize = 512

func (c *ClientSession) ReadReply() (code int, lines []string, err error) {
	for {
		line, err := c.conn.Read(MaxReplyLineSize)
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
	if len(line) < 3 {
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

func (c *ClientSession) sendCommand(cmd smtp.Command, args string) error {
	cmdString := string(cmd)
	if args != "" {
		cmdString += fmt.Sprintf(" %s", args)
	}
	return c.conn.sendRaw(cmdString)
}

func (c *ClientSession) UpgradeToTLS() {
	// Wrap the current connection with a TLS client
	tlsClient := tls.Client(c.conn.net, &tls.Config{
		InsecureSkipVerify: true, // TODO: Configure ServerName with the MX hostname
	})
	if err := tlsClient.Handshake(); err != nil {
		panic(err)
	}
	c.conn.net = tlsClient
	c.conn.reader = bufio.NewReader(c.conn.net)
	c.conn.isTLSSessionActive = true
}
