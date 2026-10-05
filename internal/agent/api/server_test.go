package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andyresta/poinhost/internal/agent/backend"
)

type fakeReader struct {
	servers []backend.ServerInfo
	status  map[string]backend.Status
}

func (f fakeReader) ListServers() ([]backend.ServerInfo, error) { return f.servers, nil }

func (f fakeReader) Server(id string) (backend.ServerInfo, bool) {
	for _, s := range f.servers {
		if s.ID == id {
			return s, true
		}
	}
	return backend.ServerInfo{}, false
}

func (f fakeReader) Status(id string) (backend.Status, bool) {
	st, ok := f.status[id]
	return st, ok
}

func (f fakeReader) Refresh(context.Context, string) (backend.Status, error) {
	return backend.Status{}, errors.New("tidak dipakai")
}

type fakeStore struct{ list []backend.Entry }

func (f fakeStore) List() ([]backend.Entry, error) { return f.list, nil }

func contoh() Deps {
	return Deps{
		Version:   "uji",
		StartedAt: time.Now().Add(-time.Minute),
		APIToken:  "token-benar",
		Reader: fakeReader{
			servers: []backend.ServerInfo{{ID: "s1", Name: "web", Host: "10.0.0.1", IsSelf: true}},
			status:  map[string]backend.Status{"s1": {Connection: "ok", CPUCores: 2}},
		},
		Servers: fakeStore{list: []backend.Entry{{ID: "s1", Name: "web"}}},
	}
}

func minta(t *testing.T, d Deps, metode, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metode, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	Handler(d).ServeHTTP(rec, req)
	return rec
}

// Semua endpoint selain /healthz harus menolak tanpa token yang cocok.
//
// Ini satu-satunya yang berdiri di antara API dan siapa pun yang berhasil
// menjangkau portnya, jadi diuji per endpoint — bukan sekali di satu
// endpoint lalu dianggap berlaku untuk semuanya. Endpoint yang lupa
// dibungkus auth() tidak akan kelihatan dengan cara lain.
func TestEndpointTerlindungMenolakTokenSalah(t *testing.T) {
	d := contoh()
	paths := []string{"/v1/status", "/v1/servers", "/v1/overview", "/v1/servers/s1/status"}

	for _, p := range paths {
		for nama, token := range map[string]string{"tanpa token": "", "token salah": "token-salah"} {
			rec := minta(t, d, http.MethodGet, p, token)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("GET %s (%s) = %d, mau 401", p, nama, rec.Code)
			}
		}
		if rec := minta(t, d, http.MethodGet, p, "token-benar"); rec.Code != http.StatusOK {
			t.Errorf("GET %s dengan token benar = %d, mau 200 (%s)", p, rec.Code, rec.Body.String())
		}
	}
}

// /healthz sengaja terbuka: app desktop memakainya untuk membedakan
// "terpasang tapi mati" dari "jalan", termasuk saat tokennya belum diset.
func TestHealthzTerbukaDanTidakMembocorkanApaPun(t *testing.T) {
	for _, d := range []Deps{contoh(), {Version: "uji", StartedAt: time.Now()}} {
		rec := minta(t, d, http.MethodGet, "/healthz", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /healthz = %d, mau 200", rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("respons bukan JSON: %v", err)
		}
		for _, k := range []string{"apiToken", "token", "servers"} {
			if _, ada := body[k]; ada {
				t.Errorf("/healthz membocorkan %q: %v", k, body)
			}
		}
	}
}

// Config tanpa apiToken adalah kegagalan konfigurasi, dan kegagalan
// konfigurasi tidak boleh berubah jadi API terbuka tanpa autentikasi.
func TestTanpaAPITokenEndpointDitutupBukanDibuka(t *testing.T) {
	d := contoh()
	d.APIToken = ""

	for _, token := range []string{"", "apa saja"} {
		rec := minta(t, d, http.MethodGet, "/v1/overview", token)
		if rec.Code == http.StatusOK {
			t.Fatalf("token kosong meloloskan permintaan: %s", rec.Body.String())
		}
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("= %d, mau 503 supaya bedanya dengan token salah terbaca", rec.Code)
		}
	}
}

// /v1/overview digabung justru supaya HP cukup satu permintaan; kalau
// metriknya hilang dari sana, HP kembali memanggil per server.
func TestOverviewMemuatServerSekaligusMetriknya(t *testing.T) {
	rec := minta(t, contoh(), http.MethodGet, "/v1/overview", "token-benar")

	var body struct {
		Servers []struct {
			ID     string         `json:"id"`
			Status map[string]any `json:"status"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respons bukan JSON: %v", err)
	}
	if len(body.Servers) != 1 || body.Servers[0].ID != "s1" {
		t.Fatalf("servers = %+v", body.Servers)
	}
	if body.Servers[0].Status["connection"] != "ok" {
		t.Errorf("metrik tidak ikut: %v", body.Servers[0].Status)
	}
}

// Nama field respons adalah kontrak dengan aplikasi yang sudah terpasang
// di HP orang, dan HP tidak ikut ter-update saat agent di-update.
func TestStatusJSONMempertahankanNamaField(t *testing.T) {
	got := statusJSON(backend.Status{Connection: "ok", CheckedAt: time.Now()})

	for _, k := range []string{
		"connection", "osName", "hostname", "uptimeSeconds", "cpuCores",
		"cpuUsedPct", "memUsedMB", "memTotalMB", "diskUsedGB", "diskTotalGB",
		"stale", "checkedAt",
	} {
		if _, ada := got[k]; !ada {
			t.Errorf("field %q hilang dari respons", k)
		}
	}
	// error hanya muncul kalau memang ada — bukan string kosong yang
	// membuat HP mengira ada masalah.
	if _, ada := got["error"]; ada {
		t.Errorf("error ikut terkirim walau kosong: %v", got)
	}
}
