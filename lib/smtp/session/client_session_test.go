package session

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const HostName = "localhost"

func setupClientSession() (*ClientSession, net.Conn, net.Conn) {
	client, server := net.Pipe()
	client.SetDeadline(time.Now().Add(time.Second))
	server.SetDeadline(time.Now().Add(time.Second))

	return NewClientSession(client, HostName), client, server
}

func TestValidateReplyLine(t *testing.T) {
	t.Parallel()
	session, client, server := setupClientSession()
	defer client.Close()
	defer server.Close()

	require.ErrorContains(t, session.validateReplyLine("12\r\n"), "cannot be less than 5 octets")

	// 3 digits is valid if
	// first digit is between 2-5
	// second digit is between 0-5
	// third digit is between 0-9
	require.NoError(t, session.validateReplyLine("239\r\n"))
	require.ErrorContains(t, session.validateReplyLine("123\r\n"), "first reply code digit must be between 2 and 5 inclusive")
	require.ErrorContains(t, session.validateReplyLine("263\r\n"), "second reply code digit must be between 0 and 5 inclusive")
	require.ErrorContains(t, session.validateReplyLine("23a\r\n"), "third reply code digit must be between 0 and 9 inclusive")

	/* see impl. comment
	// line without <CRLF> fails
	require.ErrorContains(t, session.validateReplyLine("223 asd"), "all reply lines must be terminated by <CRLF>")
	*/
	// requires either space or "-" between digits and text
	require.ErrorContains(t, session.validateReplyLine("223bad\r\n"), "reply code must be separated by space or '-'")

	require.NoError(t, session.validateReplyLine("223-good\r\n"))
	require.NoError(t, session.validateReplyLine("223 good\r\n"))
}

func TestReadReply(t *testing.T) {
	t.Parallel()

	t.Run("single line reads fine", func(t *testing.T) {
		/* conn, client, server := setupClientSession()
		defer client.Close()
		defer server.Close()

		errCh := make(chan error, 1)
		go func() {
		}() */
	})
}
