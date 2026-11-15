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

func getIpsV4() []string {
	addresses := getAddreses()
	var ips []string
	for _, adress := range addresses {
		if ipNet, ok := adress.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipV4 := ipNet.IP.To4(); ipV4 != nil {
				ips = append(ips, ipV4.String())
			}
		}
	}
	if ips == nil {
		log.Println("[ERROR] There are no IPv4 addresses")
		return []string{}
	}
	return ips
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
	ips := getIpsV4()
	author := getAuthor()

	log.Println("TEST:\n")
	log.Println(hostname)

	for _, ip := range ips {
		log.Println(ip)
	}

	log.Println(author)
}
