package main

import (
	"flag"
	"fmt"
	"log/slog"
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

	flag.Parse()

	if (hostName == nil || *hostName == "") && (sendHostName == nil || *sendHostName == "") {
		fmt.Println("hostname or send-hostname is required")
		os.Exit(1)
	}

	if (hostName != nil && *hostName != "") && (sendHostName != nil && *sendHostName != "") {
		fmt.Println("only host or send-hostname is supported")
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

	netConn, err := smtp.DialViaSocks5("127.0.0.1:1080", dialingHostName, port)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer netConn.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	session := session.NewClientSession(logger, netConn, ClientHostName)

	session.ReadGreeting()
	session.ExtendedHello()
	/* to := "receiver@example.com"
	mailTx := &smtp.MailTransaction{
		From: "sender@example.com",
		To:   &to,
	}
	session.Mail(mailTx)
	session.Recipient(mailTx) */
	session.Quit()
	os.Exit(0)
}
