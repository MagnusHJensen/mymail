package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

func main() {
	listener, err := net.Listen("tcp", ":2626")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	fmt.Printf("IMAP Server listening on port 2626\n")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Failed to accept connection: %v\n", err)
			continue
		}

		fmt.Printf("Accepted connection\n")

		// First time connection should always reply with untagged OK
		conn.Write([]byte("* OK IMAP4rev2 server ready\r\n"))

		go func(conn net.Conn) {
			defer conn.Close()
			reader := bufio.NewReader(conn)
			for {
				// Read the entire body
				line, err := reader.ReadString('\n')
				if err != nil {
					fmt.Printf("Errored on data reading: %v\n", err)
					return
				}
				line = strings.TrimRight(line, "\r\n")
				fmt.Printf("C: %s\n", line)

				if failed := handleClientCommand(conn, line); failed {
					return
				}
			}
		}(conn)
	}
}

func handleClientCommand(conn net.Conn, line string) (failed bool) {
	// client commands are always three part (for now)
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 {
		conn.Write([]byte("* BAD Request must have tag and command\r\n"))
		return false
	}
	tag, cmd := parts[0], strings.ToUpper(parts[1])

	switch cmd {
	case "CAPABILITY":
		if _, err := conn.Write([]byte("* CAPABILITY IMAP4rev2 IMAP4rev1 AUTH=PLAIN\r\n")); err != nil {
			fmt.Printf("Failed writing data: %v\n", err)
			return true
		}
	case "LOGOUT":
		if _, err := conn.Write([]byte("* BYE IMAP4rev2 Server logging out\r\n")); err != nil {
			fmt.Printf("Failed writing data: %v\n", err)
			return true
		}

		// Write tagged OK but also "failed" to indicate connection closure
		conn.Write([]byte(fmt.Sprintf("%s OK Completed\r\n", tag)))
		return true
	}

	// Default to OK for the tagged response
	conn.Write([]byte(fmt.Sprintf("%s OK Completed\r\n", tag)))
	return false
}
