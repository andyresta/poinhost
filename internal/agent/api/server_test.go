package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andyresta/poinhost/internal/agent/backend"
)

type fakeReader struct {
	self   backend.ServerInfo
	ada    bool
	status map[string]backend.Status
}

func (f fakeReader) Self() (backend.ServerInfo, bool) { return f.self, f.ada }

func (f fakeReader) Status(id string) (backend.Status, bool) {
	st, ok := f.status[id]
	return st, ok
}

func contoh() Deps {
	return Deps{
		Version:   "uji",
		StartedAt: time.Now().Add(-time.Minute),
		APIToken:  "token-benar",
		Reader: fakeReader{
			self:   backend.ServerInfo{ID: "s1", Name: "web", Host: "10.0.0.1"},
			ada:    true,
			status: map[string]backend.Status{"s1": {Connection: "ok", CPUCores: 2}},
		},
	}
}

func minta(t *testing.T, d Deps, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	Handler(d).ServeHTTP(rec, req)
	return rec
}

// Endpoint data harus menolak tanpa token yang cocok. Ini satu-satunya yang
// berdiri di antara API dan siapa pun yang berhasil menjangkau portnya.
func TestEndpointTerlindungMenolakTokenSalah(t *testing.T) {
	d := contoh()

	for nama, token := range map[string]string{"tanpa token": "", "token salah": "token-salah"} {
		if rec := minta(t, d, "/v1/server", token); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET /v1/server (%s) = %d, mau 401", nama, rec.Code)
		}
	}
	if rec := minta(t, d, "/v1/server", "token-benar"); rec.Code != http.StatusOK {
		t.Errorf("token benar ditolak: %d %s", rec.Code, rec.Body.String())
	}
}

// Rute yang sudah dibuang tidak boleh diam-diam hidup lagi. Bentuk jamaknya
// menjanjikan banyak server, padahal satu agent hanya mengurus mesinnya
// sendiri.
func TestRuteJamakSudahTidakAda(t *testing.T) {
	d := contoh()
	for _, p := range []string{"/v1/servers", "/v1/servers/s1/status", "/v1/overview", "/v1/status"} {
		if rec := minta(t, d, p, "token-benar"); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, mau 404", p, rec.Code)
		}
	}
}

// /healthz sengaja terbuka: app desktop memakainya untuk membedakan
// "terpasang tapi mati" dari "jalan", termasuk saat tokennya belum diset.
func TestHealthzTerbukaDanTidakMembocorkanApaPun(t *testing.T) {
	for _, d := range []Deps{contoh(), {Version: "uji", StartedAt: time.Now()}} {
		rec := minta(t, d, "/healthz", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /healthz = %d, mau 200", rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("respons bukan JSON: %v", err)
		}
		for _, k := range []string{"apiToken", "token", "host", "status"} {
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
		rec := minta(t, d, "/v1/server", token)
		if rec.Code == http.StatusOK {
			t.Fatalf("token kosong meloloskan permintaan: %s", rec.Body.String())
		}
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("= %d, mau 503 supaya bedanya dengan token salah terbaca", rec.Code)
		}
	}
}

// Satu permintaan harus cukup untuk menggambar layar HP: identitas mesin,
// metriknya, dan versi agent sekaligus.
func TestServer_SatuResponsMemuatSemuanya(t *testing.T) {
	rec := minta(t, contoh(), "/v1/server", "token-benar")

	var body struct {
		Version string         `json:"version"`
		ID      string         `json:"id"`
		Name    string         `json:"name"`
		Host    string         `json:"host"`
		Status  map[string]any `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respons bukan JSON: %v", err)
	}
	if body.ID != "s1" || body.Name != "web" || body.Host != "10.0.0.1" || body.Version != "uji" {
		t.Errorf("identitas = %+v", body)
	}
	if body.Status["connection"] != "ok" {
		t.Errorf("metrik tidak ikut: %v", body.Status)
	}
}

// Metrik yang belum pernah terkumpul DIHILANGKAN, bukan dikirim sebagai nol:
// layar dengan CPU 0% dan disk 0 GB terlihat seperti server sehat yang
// menganggur.
func TestServer_MetrikBelumAdaTidakDikirimSebagaiNol(t *testing.T) {
	d := contoh()
	d.Reader = fakeReader{self: backend.ServerInfo{ID: "s1", Name: "web"}, ada: true}

	rec := minta(t, d, "/v1/server", "token-benar")
	if rec.Code != http.StatusOK {
		t.Fatalf("= %d, mau 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, ada := body["status"]; ada {
		t.Errorf("status nol ikut terkirim: %v", body["status"])
	}
}

// Agent yang belum mengelola mesinnya sendiri tidak punya apa pun untuk
// dilaporkan, dan itu dibedakan dari metrik yang belum terkumpul karena
// perbaikannya berbeda.
func TestServer_TanpaSelfMembalas503(t *testing.T) {
	d := contoh()
	d.Reader = fakeReader{}

	if rec := minta(t, d, "/v1/server", "token-benar"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("= %d, mau 503", rec.Code)
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
