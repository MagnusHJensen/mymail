package smtp

import (
	"fmt"
	"io"
	"net"
)

func DialViaSocks5(proxyAddr, targetHost string, targetPort int) (net.Conn, error) {
	conn, err := net.Dial("tcp", proxyAddr) // "127.0.0.1:1080"
	if err != nil {
		return nil, err
	}

	// greeting: version 5, 1 auth method, "no auth"
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		conn.Close()
		return nil, err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil || resp[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 handshake failed")
	}

	// Resolve IPv4 locally so we control the address family — the SOCKS5
	// server's own resolver prefers AAAA, which Gmail rejects without PTR/SPF.
	ipAddr, err := net.ResolveIPAddr("ip4", targetHost)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("no IPv4 address for %s: %w", targetHost, err)
	}
	ip4 := ipAddr.IP.To4()

	req := []byte{0x05, 0x01, 0x00, 0x01} // CONNECT, IPv4 addr type
	req = append(req, ip4...)
	req = append(req, byte(targetPort>>8), byte(targetPort))
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}

	// reply: ver, rep, rsv, atyp, then variable bind addr+port
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil || head[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 connect failed: rep=%d", head[1])
	}
	switch head[3] {
	case 0x01:
		io.CopyN(io.Discard, conn, 4+2) // IPv4 + port
	case 0x04:
		io.CopyN(io.Discard, conn, 16+2) // IPv6 + port
	}

	return conn, nil
}
