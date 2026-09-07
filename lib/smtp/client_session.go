package smtp

import (
	"fmt"
	"net"
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

func (s *ClientSession) SendMail(mail *MailTransaction) error {
	if code, _, err := s.conn.ReadReply(); err != nil {
		return err
	} else if code != int(CodeReady) {
		return fmt.Errorf("Did not receive greeting code on greeting: %d", code)
	}

	if err := s.conn.SendCommand(HelloCmd, s.hostname); err != nil {
		return err
	}
	if code, _, err := s.conn.ReadReply(); err != nil {
		return err
	} else if code != int(CodeOK) {
		return fmt.Errorf("Got non OK code for %s: %d", ExtendedHelloCmd, code)
	}

	if err := s.conn.SendCommand(MailCmd, fmt.Sprintf("FROM:%s", mail.From)); err != nil {
		return err
	}
	if code, _, err := s.conn.ReadReply(); err != nil {
		return err
	} else if code != int(CodeOK) {
		return fmt.Errorf("Got non OK code for %s: %d", MailCmd, code)
	}

	if err := s.conn.SendCommand(RecipientCmd, fmt.Sprintf("TO:%s", *mail.To)); err != nil {
		return err
	}
	if code, _, err := s.conn.ReadReply(); err != nil {
		return err
	} else if code != int(CodeOK) {
		return fmt.Errorf("Got non OK code for %s: %d", RecipientCmd, code)
	}

	if err := s.conn.SendCommand(DataCmd, ""); err != nil {
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
	code, lines, err := s.conn.ReadReply()
	if err != nil {
		return err
	}
	if code != int(CodeOK) {
		return fmt.Errorf("Got non OK status code: %d\n%v\n", code, lines)
	}

	s.Quit()
	return nil
}

func (s *ClientSession) Quit() {
	s.conn.SendCommand(QuitCmd, "")
	s.conn.net.Close()
}
