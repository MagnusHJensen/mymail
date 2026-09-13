package session

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"strings"
)

// Connection represents a base over the wire SMTP connection, and provides
// low-level utilities to send and read commands.
type Connection struct {
	net    net.Conn
	reader *bufio.Reader
	logger *slog.Logger

	isTLSSessionActive bool
}

func NewConnection(net net.Conn, logger *slog.Logger) *Connection {
	return &Connection{
		net:    net,
		reader: bufio.NewReader(net),
		logger: logger,
	}
}

func (c *Connection) Read() (string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return line, fmt.Errorf("failed reading data: %w", err)
	}
	line = strings.TrimSuffix(line, "\r\n")
	c.logger.Debug(fmt.Sprintf("received %s", line))

	return line, nil
}

func (c *Connection) sendRaw(cmd string) error {
	replyLine := fmt.Sprintf("%s\r\n", cmd)
	if _, err := c.net.Write([]byte(replyLine)); err != nil {
		return err
	}
	trimmedLine := strings.TrimSuffix(replyLine, "\r\n")
	c.logger.Debug(fmt.Sprintf("sent %s", trimmedLine))
	return nil
}
