package session

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const HostName = "localhost"

func setupClientSession() (*ClientSession, net.Conn, net.Conn) {
	client, server := net.Pipe()
	client.SetDeadline(time.Now().Add(time.Second))
	server.SetDeadline(time.Now().Add(time.Second))

	return NewClientSession(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})), client, HostName), client, server
}

func generateTestCert(t *testing.T) tls.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}
}

func TestValidateReplyLine(t *testing.T) {
	t.Parallel()
	session, client, server := setupClientSession()
	defer client.Close()
	defer server.Close()

	// TODO: Figure out if we want to validate <CRLF> here
	require.ErrorContains(t, session.validateReplyLine("12"), "cannot be less than 5 octets")

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

func TestExtendedHello(t *testing.T) {
	t.Parallel()

	t.Run("successfully starts TLS when reported", func(t *testing.T) {
		session, client, server := setupClientSession()
		defer client.Close()
		defer server.Close()
		cert := generateTestCert(t)

		type observed struct {
			lines []string
			err   error
		}
		resultCh := make(chan observed, 1)
		go func() {
			var obs observed
			reader := bufio.NewReader(server)

			line, err := reader.ReadString('\n')
			obs.lines = append(obs.lines, line)
			if err != nil {
				obs.err = err
				resultCh <- obs
				return
			}
			if _, err := io.WriteString(server, "250 STARTTLS\r\n"); err != nil {
				obs.err = err
				resultCh <- obs
				return
			}

			line, err = reader.ReadString('\n')
			obs.lines = append(obs.lines, line)
			if err != nil {
				obs.err = err
				resultCh <- obs
				return
			}
			if _, err := io.WriteString(server, "220 Ready to start TLS\r\n"); err != nil {
				obs.err = err
				resultCh <- obs
				return
			}

			tlsServer := tls.Server(server, &tls.Config{Certificates: []tls.Certificate{cert}})
			server = tlsServer
			if err := tlsServer.Handshake(); err != nil {
				obs.err = err
				resultCh <- obs
				return
			}
			reader = bufio.NewReader(tlsServer)

			// Write the TLS EHLO
			line, err = reader.ReadString('\n')
			obs.lines = append(obs.lines, line)
			if err != nil {
				obs.err = err
				resultCh <- obs
				return
			}

			// Send the 250 reply
			if _, err := io.WriteString(server, "250\r\n"); err != nil {
				obs.err = err
				resultCh <- obs
				return
			}

			resultCh <- obs
		}()

		require.NoError(t, session.ExtendedHello())
		result := <-resultCh
		require.NoError(t, result.err)
		require.Equal(t, []string{"EHLO localhost\r\n", "STARTTLS\r\n", "EHLO localhost\r\n"}, result.lines)

		require.True(t, session.conn.isTLSSessionActive)
		_, isTLS := session.conn.net.(*tls.Conn)
		require.True(t, isTLS)
	})
}

func TestInitAuth(t *testing.T) {
	t.Parallel()

	t.Run("fails init auth when TLS is not configured", func(t *testing.T) {
		session, client, server := setupClientSession()
		defer client.Close()
		defer server.Close()

		require.ErrorContains(t, session.InitAuth(""), "AUTH requires an active TLS connection")
	})
}
