package smtp

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

// Connection represents a base over the wire SMTP connection, and provides
// low-level utilities to send and read commands.
type Connection struct {
	net    net.Conn
	reader *bufio.Reader

	side Side
}

type Side string

const (
	ClientSide Side = "client"
	ServerSide Side = "server"
)

func NewConnection(net net.Conn, side Side) *Connection {
	return &Connection{
		net:    net,
		reader: bufio.NewReader(net),
		side:   side,
	}
}

func (c *Connection) Read() (string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed reading data: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")
	c.logReceive(line)
	return line, nil
}

func (c *Connection) Send(code Code) error {
	return c.sendRaw(fmt.Sprintf("%d", code))
}

func (c *Connection) SendWithArgs(code Code, args string) error {
	return c.sendRaw(fmt.Sprintf("%d %s", code, args))
}

func (c *Connection) SendCommand(cmd Command, args string) error {
	return c.sendRaw(fmt.Sprintf("%s %s", string(cmd), args))
}

func (c *Connection) sendRaw(cmd string) error {
	replyLine := fmt.Sprintf("%s\r\n", cmd)
	if _, err := c.net.Write([]byte(replyLine)); err != nil {
		return err
	}
	c.logSend(replyLine)
	return nil
}

func (c *Connection) logSend(msg string) {
	if c.side == ClientSide {
		fmt.Printf("C: %s\n", msg)
	} else {
		fmt.Printf("S: %s\n", msg)
	}
}

func (c *Connection) logReceive(msg string) {
	if c.side == ClientSide {
		fmt.Printf("S: %s\n", msg)
	} else {
		fmt.Printf("C: %s\n", msg)
	}
}
