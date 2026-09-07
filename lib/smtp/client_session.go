package smtp

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

type ClientSession struct {
	conn *Connection
}

func NewClientSession(netConn net.Conn) *ClientSession {
	return &ClientSession{
		conn: NewConnection(netConn, ClientSide),
	}
}

func (s *ClientSession) SendMail(mail *MailTransaction) error {
	if _, err := s.conn.Read(); err != nil {
		return err
	}

	if err := s.conn.SendCommand(ExtendedHelloCmd, "localhost"); err != nil {
		return err
	}
	if _, err := s.conn.Read(); err != nil {
		return err
	}

	if err := s.conn.SendCommand(MailCmd, fmt.Sprintf("FROM:%s", mail.From)); err != nil {
		return err
	}
	if _, err := s.conn.Read(); err != nil {
		return err
	}

	if err := s.conn.SendCommand(RecipientCmd, fmt.Sprintf("TO:%s", *mail.To)); err != nil {
		return err
	}
	if _, err := s.conn.Read(); err != nil {
		return err
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

	if err := s.conn.sendRaw("\r\n.\r\n"); err != nil {
		return err
	}
	dataReply, err := s.conn.Read()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(dataReply, fmt.Sprintf("%d", CodeOK)) {
		return errors.New("Got non OK status code: " + dataReply)
	}

	s.Quit()
	return nil
}

func (s *ClientSession) Quit() {
	s.conn.SendCommand(QuitCmd, "")
	s.conn.net.Close()
}
