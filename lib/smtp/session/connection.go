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

func (c *Connection) Read(maxSize int) (string, error) {
	var buf []byte
	for {
		b, err := c.reader.ReadByte()
		if err != nil {
			return string(buf), err
		}
		buf = append(buf, b)
		if len(buf) > maxSize-2 { // -2 to accomodate for the <CRLF>
			return "", fmt.Errorf("line exceeds max length of %d bytes", maxSize)
		}

		// After each byte read, peek the next two to see if its a CRLF sequence.
		nextBytes, err := c.reader.Peek(2)
		if err != nil {
			// Whatever we do peek even if not both bytes, store it in the buffer
			if len(nextBytes) > 0 {
				buf = append(buf, nextBytes...)
			}
			return string(buf), err
		}
		if nextBytes[0] == '\r' && nextBytes[1] == '\n' {
			// no \r\n to trim anymore.
			// but advance the reader to avoid next read to start with these bytes
			c.reader.Discard(2)
			c.logger.Debug(fmt.Sprintf("received %s", string(buf)))
			return string(buf), nil
		}
	}
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
