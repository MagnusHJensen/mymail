package main

import (
	"fmt"
	"net"
)

func main() {
	mxs, _ := net.LookupMX("byranlev.dk")

	for _, mx := range mxs {
		fmt.Printf("%+v\n", mx)
	}
}
