package main

import (
	"log"
	"net"
)

func main() {
	addresses, er := net.InterfaceAddrs()
	if er != nil {
		log.Println("Addresses identify error")
		return
	}
	for a, b := range addresses {
		log.Println(a, b)
	}
}
