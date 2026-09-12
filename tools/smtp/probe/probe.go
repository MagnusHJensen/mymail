package main

import (
	"flag"
	"fmt"
	"os"

	"dk.magnusjensen/mymail/lib/smtp"
	"dk.magnusjensen/mymail/lib/smtp/session"
)

// This is a local tool to probe other SMTP servers for their supported extensions
// https://www.iana.org/assignments/smtp#smtp-service-extensions

const ClientHostName = "example.com"

func main() {
	hostName := flag.String("hostname", "", "The hostname to probe, not the MX record.")

	flag.Parse()

	if hostName == nil || *hostName == "" {
		fmt.Println("hostname is required")
		os.Exit(1)
	}

	mxHost, err := smtp.GetPreferredMXHost(*hostName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	netConn, err := smtp.DialViaSocks5("127.0.0.1:1080", mxHost, 25)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer netConn.Close()

	session := session.NewClientSession(netConn, ClientHostName)

	session.ReadGreeting()
	session.ExtendedHello()
	session.Quit()
	os.Exit(0)
}
