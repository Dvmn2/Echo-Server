package main

import (
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	listenAddr = ":8000"
	refreshTTL = 5 * time.Second
)

var (
	mu          sync.Mutex
	data        info
	lastRefresh time.Time
)

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

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Printf("[ERROR] Host name identify error: %v", err)
		return "Unspecified"
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
		return []string{"Unspecified"}
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

func getData() info {
	mu.Lock()
	defer mu.Unlock()

	if time.Since(lastRefresh) >= refreshTTL {
		data = loadData()
		lastRefresh = time.Now()
	}
	return data
}

func loadData() info {
	return info{
		Hostname: getHostname(),
		IP:       strings.Join(getIpsV4(), ", "),
		Author:   getAuthor(),
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.Execute(w, getData()); err != nil {
		log.Printf("[ERROR] Render error: %v", err)
	}
}

func main() {
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}
