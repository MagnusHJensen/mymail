package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"

	"dk.magnusjensen/mymail/lib/smtp"
	"dk.magnusjensen/mymail/lib/smtp/session"
)

// This is a local tool to probe other SMTP servers for their supported extensions
// https://www.iana.org/assignments/smtp#smtp-service-extensions

const ClientHostName = "example.com"

func main() {
	hostName := flag.String("hostname", "", "The hostname to probe, not the MX record.")
	sendHostName := flag.String("send-hostname", "", "The hostname to probe for sending")
	username := flag.String("username", "", "The username to use for PLAIN auth on send hostname")
	password := flag.String("password", "", "The password to use for PLAIN auth on send hostname")

	flag.Parse()

	if (hostName == nil || *hostName == "") && (sendHostName == nil || *sendHostName == "") {
		fmt.Println("hostname or send-hostname is required")
		os.Exit(1)
	}

	if (hostName != nil && *hostName != "") && (sendHostName != nil && *sendHostName != "") {
		fmt.Println("only host or send-hostname is supported")
		os.Exit(1)
	}

	if sendHostName != nil && *sendHostName != "" && (username == nil || *username == "" || password == nil || *password == "") {
		fmt.Println("username and password is required for send host")
		os.Exit(1)
	}

	var dialingHostName string
	var port int
	if *hostName != "" {
		mxHost, err := smtp.GetPreferredMXHost(*hostName)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		dialingHostName = mxHost
		port = 25
	} else {
		dialingHostName = *sendHostName
		port = 587
	}

	var netConn net.Conn
	if dialingHostName == "localhost" || dialingHostName == "127.0.0.1" {
		conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", dialingHostName, port))
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		netConn = conn
	} else {
		sockConn, err := smtp.DialViaSocks5("127.0.0.1:1080", dialingHostName, port)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		netConn = sockConn
	}

	defer netConn.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var opts []session.ClientSessionOption
	if sendHostName != nil && *sendHostName != "" {
		opts = append(opts, session.WithPlainAuth(*username, *password))
	}
	session := session.NewClientSession(logger, netConn, ClientHostName, opts...)

	session.ReadGreeting()
	session.ExtendedHello()
	to := "receiver@example.com"
	mailTx := &smtp.MailTransaction{
		From: "sender@example.com",
		To:   &to,
	}

	session.Mail(mailTx)
	session.Recipient(mailTx)
	session.Quit()
	os.Exit(0)
}
