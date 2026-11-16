package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// withData подменяет глобальную переменную data на время теста.
func withData(t *testing.T, d info) {
	t.Helper()
	old := data
	data = d
	t.Cleanup(func() { data = old })
}

func TestGetAuthor(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{"задан", "Ivan", "Ivan"},
		{"пустой", "", "Unspecified"},
		{"юникод", "Иван Петров", "Иван Петров"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AUTHOR", tt.env)
			if got := getAuthor(); got != tt.want {
				t.Errorf("getAuthor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetAuthor_NotSet(t *testing.T) {
	old, existed := os.LookupEnv("AUTHOR")
	os.Unsetenv("AUTHOR")
	t.Cleanup(func() {
		if existed {
			os.Setenv("AUTHOR", old)
		}
	})

	if got := getAuthor(); got != "Unspecified" {
		t.Errorf("getAuthor() = %q, want %q", got, "Unspecified")
	}
}

func TestGetHostname(t *testing.T) {
	want, err := os.Hostname()
	if err != nil {
		t.Skipf("os.Hostname недоступен: %v", err)
	}
	if got := getHostname(); got != want {
		t.Errorf("getHostname() = %q, want %q", got, want)
	}
}

func TestGetAddreses_NotNil(t *testing.T) {
	if getAddreses() == nil {
		t.Error("getAddreses() вернула nil, ожидался непустой срез или пустой срез")
	}
}

func TestGetIpsV4(t *testing.T) {
	ips := getIpsV4()
	if ips == nil {
		t.Fatal("getIpsV4() вернула nil, ожидался срез (возможно, пустой)")
	}
	for _, s := range ips {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Errorf("%q не является корректным IP-адресом", s)
			continue
		}
		if ip.To4() == nil {
			t.Errorf("%q не IPv4", s)
		}
		if ip.IsLoopback() {
			t.Errorf("%q — loopback-адрес, он должен быть исключён", s)
		}
		if strings.Contains(s, ":") {
			t.Errorf("%q похож на IPv6", s)
		}
	}
}

func TestLoadData(t *testing.T) {
	t.Setenv("AUTHOR", "Tester")
	withData(t, info{})

	loadData()

	if data.Author != "Tester" {
		t.Errorf("Author = %q, want %q", data.Author, "Tester")
	}
	if want, err := os.Hostname(); err == nil && data.Hostname != want {
		t.Errorf("Hostname = %q, want %q", data.Hostname, want)
	}
	if want := strings.Join(getIpsV4(), ", "); data.IP != want {
		t.Errorf("IP = %q, want %q", data.IP, want)
	}
}

func TestLoadData_DefaultAuthor(t *testing.T) {
	t.Setenv("AUTHOR", "")
	withData(t, info{})

	loadData()

	if data.Author != "Unspecified" {
		t.Errorf("Author = %q, want %q", data.Author, "Unspecified")
	}
}

func TestHandler_OK(t *testing.T) {
	withData(t, info{Hostname: "test-host", IP: "10.0.0.1, 192.168.1.5", Author: "Tester"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/html; charset=utf-8")
	}

	body := rec.Body.String()
	for _, want := range []string{
		"Информация о хосте",
		"Имя хоста:",
		"IP-адрес:",
		"Автор:",
		"test-host",
		"10.0.0.1, 192.168.1.5",
		"Tester",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("в теле ответа нет %q\nтело:\n%s", want, body)
		}
	}
}

func TestHandler_NotFound(t *testing.T) {
	withData(t, info{})

	for _, path := range []string{"/foo", "/index.html", "/a/b", "/favicon.ico"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			handler(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHandler_EscapesHTML(t *testing.T) {
	withData(t, info{
		Hostname: "<b>host</b>",
		IP:       "1.1.1.1",
		Author:   `<script>alert("xss")</script>`,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "<script>") {
		t.Error("тело содержит неэкранированный <script>")
	}
	if strings.Contains(body, "<b>host</b>") {
		t.Error("тело содержит неэкранированный <b>")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("ожидалось экранированное значение автора (&lt;script&gt;)")
	}
}

func TestHandler_EmptyData(t *testing.T) {
	withData(t, info{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<table>") {
		t.Error("страница должна отрисоваться даже при пустых данных")
	}
}

func TestHandler_WithMux(t *testing.T) {
	withData(t, info{Hostname: "mux-host", IP: "127.0.0.2", Author: "Mux"})

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("GET / status = %d, want %d", res.StatusCode, http.StatusOK)
	}

	res2, err := http.Get(srv.URL + "/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusNotFound {
		t.Errorf("GET /nope status = %d, want %d", res2.StatusCode, http.StatusNotFound)
	}
}
