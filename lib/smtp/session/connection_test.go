package session

import (
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func setupConnection() (*Connection, net.Conn, net.Conn) {
	conn1, conn2 := net.Pipe()
	// add 1 second deadline for both connections to avoid hanging up tests
	conn1.SetDeadline(time.Now().Add(time.Second))
	conn2.SetDeadline(time.Now().Add(time.Second))

	connection := NewConnection(conn1, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return connection, conn1, conn2
}

func TestConnectionRead(t *testing.T) {
	t.Parallel()

	t.Run("successfully reads <CRLF> terminated lines", func(t *testing.T) {
		t.Parallel()
		connection, client, server := setupConnection()
		defer client.Close()
		defer server.Close()
		go func() {
			_, err := io.WriteString(server, "First line\r\n")
			require.NoError(t, err)
			_, err = io.WriteString(server, "Second line\r\n")
			require.NoError(t, err)
		}()

		// client read, reads a single line
		line, err := connection.Read(1000)
		require.NoError(t, err)
		require.Equal(t, "First line", line)
		line, err = connection.Read(1000)
		require.NoError(t, err)
		require.Equal(t, "Second line", line)
	})

	t.Run("requires <CRLF> terminated lines", func(t *testing.T) {
		t.Parallel()
		connection, client, server := setupConnection()
		defer client.Close()
		defer server.Close()

		go func() {
			_, err := io.WriteString(server, "Multi")
			require.NoError(t, err)
			_, err = io.WriteString(server, "Line\n")
			require.NoError(t, err)
		}()

		require.NoError(t, client.SetReadDeadline(time.Now().Add(time.Millisecond*100)))
		line, err := connection.Read(1000)
		require.ErrorContains(t, err, "i/o timeout")
		require.Equal(t, "MultiLine\n", line) // We get both writes without CRLF and read timeout.
	})

	t.Run("caps message read size", func(t *testing.T) {
		t.Parallel()
		connection, client, server := setupConnection()
		defer client.Close()
		defer server.Close()

		go func() {
			io.WriteString(server, "Multi")
		}()

		line, err := connection.Read(2)
		require.ErrorContains(t, err, "line exceeds max length")
		require.Empty(t, line)
	})

}

func TestConnectionSendRaw(t *testing.T) {
	t.Parallel()
	connection, client, server := setupConnection()
	defer client.Close()
	defer server.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- connection.sendRaw("Custom line")
	}()

	buf := make([]byte, 64)
	n, err := server.Read(buf)
	require.NoError(t, err)

	require.NoError(t, <-errCh)
	require.Equal(t, "Custom line\r\n", string(buf[:n]))
}
