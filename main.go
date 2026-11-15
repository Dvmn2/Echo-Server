package main

import (
	"log"
	"net"
	"os"
)

func main() {
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("Host name identify error")
		return
	}
	log.Println(hostname)
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		log.Println("Addresses identify error")
		return
	}
	for a, b := range addresses {
		log.Println(a, b)
	}
}
