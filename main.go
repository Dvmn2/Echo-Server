package main

import (
	"log"
	"net"
	"os"
)

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("[ERROR] Host name identify error")
		return ""
	}
	return hostname
}

func getAddreses() []net.Addr {

	addresses, err := net.InterfaceAddrs()
	if err != nil {
		log.Println("[ERROR] Addresses identify error")
		return []net.Addr{}
	}
	return addresses
}

func getAuthor() string {
	author := os.Getenv("AUTHOR")
	if author == "" {
		log.Println("[WARNING] Author is not specified")
		author = "Unspecified"
	}
	return author
}

func main() {
	hostname := getHostname()
	addresses := getAddreses()
	author := getAuthor()

	log.Println("TEST:\n")
	log.Println(hostname)

	for _, address := range addresses {
		log.Println(address)
	}

	log.Println(author)
}
