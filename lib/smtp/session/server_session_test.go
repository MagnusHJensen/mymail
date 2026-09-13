package session

import (
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"dk.magnusjensen/mymail/lib/smtp"
	"github.com/stretchr/testify/require"
)

type mockServerSessionHandler struct {
}

func (ms *mockServerSessionHandler) QueueMail(mail *smtp.MailTransaction) error {
	return nil
}

func (ms *mockServerSessionHandler) AuthenticateUser(username, password string) error {
	return nil
}

func setupServerSession(serverType ServerType) (*ServerSession, net.Conn, net.Conn) {
	client, server := net.Pipe()
	client.SetDeadline(time.Now().Add(time.Second))
	server.SetDeadline(time.Now().Add(time.Second))

	return NewServerSession(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})), server, &mockServerSessionHandler{}, serverType, HostName), client, server
}

func TestReplyMulti(t *testing.T) {
	t.Parallel()
	runTest := func(reply, expected []string) {
		session, client, server := setupServerSession(SendServerType)
		defer client.Close()
		defer server.Close()

		type observed struct {
			code  int
			lines []string
			err   error
		}
		resultCh := make(chan observed, 1)
		go func() {
			var obs observed
			clientSession := NewClientSession(slog.Default(), client, HostName)

			code, lines, err := clientSession.ReadReply()
			obs.code = code
			obs.lines = append(obs.lines, lines...)
			if err != nil {
				obs.err = err
				resultCh <- obs
				return
			}

			resultCh <- obs
			return
		}()

		require.NoError(t, session.ReplyMulti(smtp.CodeOK, reply))

		result := <-resultCh
		require.NoError(t, result.err)
		require.Equal(t, smtp.CodeOK, smtp.Code(result.code))
		require.Equal(t, expected, result.lines)
	}

	t.Run("multi reply formats correctly", func(t *testing.T) {
		runTest([]string{"STARTTLS", "AUTH PLAIN"}, []string{"250-STARTTLS", "250 AUTH PLAIN"})
	})

	t.Run("single line formats correctly", func(t *testing.T) {
		runTest([]string{"STARTTLS"}, []string{"250 STARTTLS"})
	})
}
