package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- вспомогательные функции для работы с кэшем ----------

// setCache подменяет кэш (data и lastRefresh) под мьютексом
// и восстанавливает прежнее состояние после теста.
func setCache(t *testing.T, d info, fetchedAt time.Time) {
	t.Helper()

	mu.Lock()
	oldData, oldTime := data, lastRefresh
	data, lastRefresh = d, fetchedAt
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		data, lastRefresh = oldData, oldTime
		mu.Unlock()
	})
}

// withFreshCache кладёт в кэш d как только что полученные данные:
// getData() вернёт их без пересбора.
func withFreshCache(t *testing.T, d info) {
	t.Helper()
	setCache(t, d, time.Now())
}

// withEmptyCache делает кэш «никогда не заполнявшимся»:
// следующий getData() обязан собрать данные заново.
func withEmptyCache(t *testing.T) {
	t.Helper()
	setCache(t, info{}, time.Time{})
}

// expireCache искусственно «старит» кэш, чтобы следующий getData() обновил его.
func expireCache() {
	mu.Lock()
	lastRefresh = time.Now().Add(-refreshTTL - time.Second)
	mu.Unlock()
}

func get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// ---------- сбор отдельных значений ----------

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
	// t.Setenv запоминает исходное значение и восстановит его при завершении теста.
	t.Setenv("AUTHOR", "x")
	os.Unsetenv("AUTHOR")

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
		t.Error("getAddreses() вернула nil, ожидался срез (возможно, пустой)")
	}
}

func TestGetIpsV4(t *testing.T) {
	ips := getIpsV4()
	if len(ips) == 0 {
		t.Fatal("getIpsV4() вернула пустой срез, ожидался хотя бы один элемент (адрес или заглушка)")
	}

	// Если IPv4-адресов нет, функция возвращает заглушку "Unspecified".
	if len(ips) == 1 && ips[0] == "Unspecified" {
		t.Log("IPv4-адресов нет, получена заглушка")
		return
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

// ---------- loadData: теперь возвращает info и не трогает кэш ----------

func TestLoadData(t *testing.T) {
	t.Setenv("AUTHOR", "Tester")

	got := loadData()

	if got.Author != "Tester" {
		t.Errorf("Author = %q, want %q", got.Author, "Tester")
	}
	if want, err := os.Hostname(); err == nil && got.Hostname != want {
		t.Errorf("Hostname = %q, want %q", got.Hostname, want)
	}
	if want := strings.Join(getIpsV4(), ", "); got.IP != want {
		t.Errorf("IP = %q, want %q", got.IP, want)
	}
}

func TestLoadData_DefaultAuthor(t *testing.T) {
	t.Setenv("AUTHOR", "")

	if got := loadData(); got.Author != "Unspecified" {
		t.Errorf("Author = %q, want %q", got.Author, "Unspecified")
	}
}

func TestLoadData_DoesNotTouchCache(t *testing.T) {
	want := info{Hostname: "cached", IP: "1.2.3.4", Author: "Cached"}
	withFreshCache(t, want)
	t.Setenv("AUTHOR", "Other")

	loadData()

	mu.Lock()
	defer mu.Unlock()
	if data != want {
		t.Errorf("loadData изменила кэш: data = %+v, want %+v", data, want)
	}
}

// ---------- getData: логика кэша с TTL ----------

func TestGetData_FirstCallLoads(t *testing.T) {
	t.Setenv("AUTHOR", "First")
	withEmptyCache(t)

	got := getData()

	if got.Author != "First" {
		t.Errorf("Author = %q, want %q", got.Author, "First")
	}
	if got.Hostname == "" && got.IP == "" {
		t.Error("при первом вызове данные должны быть собраны")
	}

	mu.Lock()
	defer mu.Unlock()
	if data != got {
		t.Errorf("кэш не обновлён: data = %+v, want %+v", data, got)
	}
	if time.Since(lastRefresh) >= refreshTTL {
		t.Error("lastRefresh не обновлён после сбора данных")
	}
}

func TestGetData_UsesCacheWithinTTL(t *testing.T) {
	cached := info{Hostname: "cached-host", IP: "10.0.0.1", Author: "Cached"}
	withFreshCache(t, cached)
	// Если бы данные пересобирались, Author стал бы "Changed".
	t.Setenv("AUTHOR", "Changed")

	if got := getData(); got != cached {
		t.Errorf("getData() = %+v, want закэшированное %+v", got, cached)
	}
}

func TestGetData_JustBeforeTTLStillCached(t *testing.T) {
	cached := info{Hostname: "h", IP: "10.0.0.1", Author: "Cached"}
	// Данные получены за секунду до истечения TTL.
	setCache(t, cached, time.Now().Add(-(refreshTTL - time.Second)))
	t.Setenv("AUTHOR", "Changed")

	if got := getData(); got != cached {
		t.Errorf("getData() = %+v, want закэшированное %+v", got, cached)
	}
}

func TestGetData_RefreshesAfterTTL(t *testing.T) {
	withFreshCache(t, info{Hostname: "old", IP: "10.0.0.1", Author: "Old"})
	t.Setenv("AUTHOR", "New")
	expireCache()

	got := getData()

	if got.Author != "New" {
		t.Errorf("Author = %q, want %q (кэш должен был обновиться)", got.Author, "New")
	}
	if got.Hostname == "old" {
		t.Error("Hostname остался старым, кэш не обновился")
	}

	mu.Lock()
	defer mu.Unlock()
	if time.Since(lastRefresh) >= refreshTTL {
		t.Error("lastRefresh не сдвинулся после обновления")
	}
}

func TestGetData_TTLBoundaryRefreshes(t *testing.T) {
	// Условие обновления нестрогое (>=): ровно refreshTTL назад уже считается устаревшим.
	setCache(t, info{Author: "Old"}, time.Now().Add(-refreshTTL))
	t.Setenv("AUTHOR", "New")

	if got := getData(); got.Author != "New" {
		t.Errorf("Author = %q, want %q", got.Author, "New")
	}
}

func TestGetData_FullCycle(t *testing.T) {
	withEmptyCache(t)

	t.Setenv("AUTHOR", "A")
	if got := getData(); got.Author != "A" {
		t.Fatalf("1: Author = %q, want %q", got.Author, "A")
	}

	// В пределах TTL изменение окружения не видно.
	t.Setenv("AUTHOR", "B")
	if got := getData(); got.Author != "A" {
		t.Fatalf("2: Author = %q, want %q (кэш)", got.Author, "A")
	}

	// После истечения TTL данные обновляются.
	expireCache()
	if got := getData(); got.Author != "B" {
		t.Fatalf("3: Author = %q, want %q (обновление)", got.Author, "B")
	}
}

// Запускать с -race: проверяет отсутствие гонок на data и lastRefresh.
func TestGetData_Concurrent(t *testing.T) {
	t.Setenv("AUTHOR", "Concurrent")
	withEmptyCache(t)

	const goroutines = 50
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if i == 0 && j%5 == 0 {
					expireCache() // периодически форсируем обновление
				}
				if got := getData(); got.Author != "Concurrent" {
					t.Errorf("Author = %q, want %q", got.Author, "Concurrent")
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

// ---------- handler ----------

func TestHandler_OK(t *testing.T) {
	withFreshCache(t, info{Hostname: "test-host", IP: "10.0.0.1, 192.168.1.5", Author: "Tester"})

	rec := get(t, "/")
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
	withFreshCache(t, info{})

	for _, path := range []string{"/foo", "/index.html", "/a/b", "/favicon.ico"} {
		t.Run(path, func(t *testing.T) {
			if rec := get(t, path); rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHandler_NotFoundDoesNotRefresh(t *testing.T) {
	cached := info{Hostname: "h", IP: "10.0.0.1", Author: "Cached"}
	withFreshCache(t, cached)
	expireCache()
	t.Setenv("AUTHOR", "Changed")

	get(t, "/nope")

	mu.Lock()
	defer mu.Unlock()
	if data != cached {
		t.Errorf("запрос к несуществующему пути обновил кэш: %+v", data)
	}
}

func TestHandler_EscapesHTML(t *testing.T) {
	withFreshCache(t, info{
		Hostname: "<b>host</b>",
		IP:       "1.1.1.1",
		Author:   `<script>alert("xss")</script>`,
	})

	body := get(t, "/").Body.String()
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
	withFreshCache(t, info{})

	rec := get(t, "/")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<table>") {
		t.Error("страница должна отрисоваться даже при пустых данных")
	}
}

func TestHandler_LoadsDataOnFirstRequest(t *testing.T) {
	t.Setenv("AUTHOR", "FromEnv")
	withEmptyCache(t)

	body := get(t, "/").Body.String()
	if !strings.Contains(body, "FromEnv") {
		t.Errorf("первый запрос должен собрать данные, в теле нет автора:\n%s", body)
	}
}

func TestHandler_RefreshesPerTTL(t *testing.T) {
	withEmptyCache(t)

	t.Setenv("AUTHOR", "First")
	if body := get(t, "/").Body.String(); !strings.Contains(body, "First") {
		t.Fatalf("1: ожидался автор First:\n%s", body)
	}

	// Повторный запрос в пределах TTL отдаёт старые данные.
	t.Setenv("AUTHOR", "Second")
	body := get(t, "/").Body.String()
	if !strings.Contains(body, "First") || strings.Contains(body, "Second") {
		t.Fatalf("2: ожидались закэшированные данные (First):\n%s", body)
	}

	// После истечения TTL запрос обновляет данные.
	expireCache()
	body = get(t, "/").Body.String()
	if !strings.Contains(body, "Second") || strings.Contains(body, "First") {
		t.Fatalf("3: ожидались обновлённые данные (Second):\n%s", body)
	}
}

func TestHandler_Concurrent(t *testing.T) {
	t.Setenv("AUTHOR", "Parallel")
	withEmptyCache(t)

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := get(t, "/")
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if !strings.Contains(rec.Body.String(), "Parallel") {
				t.Error("в теле нет автора Parallel")
			}
		}()
	}
	wg.Wait()
}

func TestHandler_WithMux(t *testing.T) {
	withFreshCache(t, info{Hostname: "mux-host", IP: "127.0.0.2", Author: "Mux"})

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
