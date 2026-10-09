package main

import (
	"fmt"
	"log"
	"net"

	"dk.magnusjensen/mymail/lib/smtp/spf"
)

const Hostname = "_spf.google.com"

func main() {
	//myIP := GetOutboundIP()
	result, err := spf.VerifySPFHost("209.85.128.0", Hostname)
	if err != nil {
		panic(err)
	}
	fmt.Printf("SPF result: %s\n", result)
	mxs, _ := net.LookupTXT(Hostname)

	for _, mx := range mxs {
		fmt.Printf("%+v\n", mx)
	}
}

func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}
