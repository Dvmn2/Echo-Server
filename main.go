package main

import (
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

const listenAddr = ":8000"

var page = template.Must(template.New("page").Parse(`
<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <title>Информация о хосте</title>
</head>
<body>
  <h1>Информация о хосте</h1>
  <table>
    <tr><td>Имя хоста:</td><td>{{.Hostname}}</td></tr>
    <tr><td>IP-адрес:</td><td>{{.IP}}</td></tr>
    <tr><td>Автор:</td><td>{{.Author}}</td></tr>
  </table>
</body>
</html>
`))

type info struct {
	Hostname string
	IP       string
	Author   string
}

var data info

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("[ERROR] Host name identify error: %v", err)
		return ""
	}
	return hostname
}

func getAddreses() []net.Addr {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		log.Printf("[ERROR] Addresses identify error: %v", err)
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
		log.Println("[WARNING] There are no IPv4 addresses")
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

func loadData() {
	hostname := getHostname()
	ips := strings.Join(getIpsV4(), ", ")
	author := getAuthor()
	data = info{Hostname: hostname, IP: ips, Author: author}
}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.Execute(w, data); err != nil {
		log.Printf("[ERROR] Render error: %v", err)
	}
}

func main() {
	loadData()
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}
