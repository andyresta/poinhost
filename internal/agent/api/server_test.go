package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andyresta/poinhost/internal/agent/backend"
	"github.com/andyresta/poinhost/internal/agent/devices"
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

func contoh(t *testing.T) (Deps, *devices.Store) {
	t.Helper()
	store, err := devices.Open(filepath.Join(t.TempDir(), "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	return Deps{
		Version:     "uji",
		StartedAt:   time.Now().Add(-time.Minute),
		EnrollToken: "token-daftar",
		Reader: fakeReader{
			self:   backend.ServerInfo{ID: "s1", Name: "web", Host: "10.0.0.1"},
			ada:    true,
			status: map[string]backend.Status{"s1": {Connection: "ok", CPUCores: 2}},
		},
		Devices: store,
	}, store
}

func minta(t *testing.T, h http.Handler, metode, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(metode, path, nil)
	} else {
		r = httptest.NewRequest(metode, path, strings.NewReader(body))
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// daftar menjalankan langkah 1-2 alurnya: HP menukar token pendaftaran
// dengan token miliknya sendiri.
func daftar(t *testing.T, h http.Handler, nama string) (deviceID, token string) {
	t.Helper()
	rec := minta(t, h, http.MethodPost, "/v1/devices", "token-daftar", `{"name":"`+nama+`","platform":"android"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("daftar = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		DeviceID string `json:"deviceId"`
		Token    string `json:"token"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("respons bukan JSON: %v", err)
	}
	if body.Status != devices.StatusPending {
		t.Errorf("status setelah daftar = %q, mau pending", body.Status)
	}
	if body.Token == "" || body.DeviceID == "" {
		t.Fatalf("respons tidak lengkap: %+v", body)
	}
	return body.DeviceID, body.Token
}

// Inti alurnya: token yang baru didaftarkan BELUM membuka apa pun, dan baru
// berlaku setelah disetujui dari app desktop.
func TestAlurPersetujuan(t *testing.T) {
	d, store := contoh(t)
	pub := Handler(d)
	admin := AdminHandler(AdminDeps{Devices: store})

	id, token := daftar(t, pub, "HP Andy")

	rec := minta(t, pub, http.MethodGet, "/v1/server", token, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sebelum disetujui = %d, mau 403", rec.Code)
	}
	// HP perlu bisa membedakan "tunggu" dari "token salah", supaya layarnya
	// bisa bilang menunggu alih-alih menyuruh daftar ulang.
	var pending map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &pending)
	if pending["status"] != devices.StatusPending {
		t.Errorf("status = %q, mau pending", pending["status"])
	}

	if rec := minta(t, admin, http.MethodPost, "/admin/devices/"+id+"/approve", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", rec.Code, rec.Body.String())
	}

	if rec := minta(t, pub, http.MethodGet, "/v1/server", token, ""); rec.Code != http.StatusOK {
		t.Fatalf("setelah disetujui = %d: %s", rec.Code, rec.Body.String())
	}

	// Dicabut: dibedakan dari token asing, supaya HP bisa bilang aksesnya
	// diambil, bukan menyuruh mengulang pendaftaran yang akan ditolak lagi.
	if rec := minta(t, admin, http.MethodPost, "/admin/devices/"+id+"/revoke", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d", rec.Code)
	}
	rec = minta(t, pub, http.MethodGet, "/v1/server", token, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("setelah dicabut = %d, mau 403", rec.Code)
	}
	var dicabut map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &dicabut)
	if dicabut["status"] != devices.StatusRevoked {
		t.Errorf("status = %q, mau revoked", dicabut["status"])
	}
}

// Mencabut satu HP tidak boleh memutus yang lain — inilah alasan tokennya
// per perangkat, bukan satu token statis untuk semuanya.
func TestCabutSatuTidakMemutusYangLain(t *testing.T) {
	d, store := contoh(t)
	pub := Handler(d)
	admin := AdminHandler(AdminDeps{Devices: store})

	id1, token1 := daftar(t, pub, "HP lama")
	_, token2 := daftar(t, pub, "HP baru")
	for _, dev := range store.List() {
		if _, err := store.SetStatus(dev.ID, devices.StatusApproved); err != nil {
			t.Fatal(err)
		}
	}

	if rec := minta(t, admin, http.MethodPost, "/admin/devices/"+id1+"/revoke", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d", rec.Code)
	}
	if rec := minta(t, pub, http.MethodGet, "/v1/server", token1, ""); rec.Code != http.StatusForbidden {
		t.Errorf("HP yang dicabut masih bisa akses: %d", rec.Code)
	}
	if rec := minta(t, pub, http.MethodGet, "/v1/server", token2, ""); rec.Code != http.StatusOK {
		t.Errorf("HP lain ikut terputus: %d", rec.Code)
	}
}

// Token pendaftaran BUKAN token akses, dan token perangkat tidak bisa
// mendaftarkan perangkat lain. Kalau keduanya tertukar, seluruh gunanya
// persetujuan hilang: siapa pun yang punya token pendaftaran langsung bisa
// membaca server tanpa menunggu siapa pun.
func TestKeduaTokenTidakBolehSalingDipakai(t *testing.T) {
	d, store := contoh(t)
	pub := Handler(d)

	if rec := minta(t, pub, http.MethodGet, "/v1/server", "token-daftar", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("token pendaftaran membaca data: %d %s", rec.Code, rec.Body.String())
	}

	_, token := daftar(t, pub, "HP")
	if _, err := store.SetStatus(store.List()[0].ID, devices.StatusApproved); err != nil {
		t.Fatal(err)
	}
	if rec := minta(t, pub, http.MethodPost, "/v1/devices", token, `{"name":"HP lain"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("token perangkat mendaftarkan perangkat lain: %d", rec.Code)
	}
}

// Token perangkat disimpan sebagai hash. Berkasnya ikut tersalin ke backup
// mana pun yang menyentuh direktori data agent, jadi token apa adanya di sana
// berarti setiap salinan berkas itu adalah kunci yang masih hidup.
func TestTokenPerangkatTidakDisimpanApaAdanya(t *testing.T) {
	d, store := contoh(t)
	pub := Handler(d)

	_, token := daftar(t, pub, "HP")
	for _, dev := range store.List() {
		if strings.Contains(dev.TokenHash, token) || dev.TokenHash == token {
			t.Fatal("token tersimpan apa adanya")
		}
		if dev.TokenHash == "" {
			t.Fatal("tidak ada hash tersimpan — token tidak akan pernah cocok")
		}
	}
}

// Daftar yang keluar dari agent tidak membawa hash token ke layar desktop
// maupun ke log mana pun yang merekam respons ini.
func TestAdminTidakMengirimHashToken(t *testing.T) {
	d, store := contoh(t)
	daftar(t, Handler(d), "HP")

	rec := minta(t, AdminHandler(AdminDeps{Devices: store}), http.MethodGet, "/admin/devices", "", "")
	if strings.Contains(rec.Body.String(), "tokenHash\":\"") &&
		!strings.Contains(rec.Body.String(), `"tokenHash":""`) {
		t.Errorf("hash ikut terkirim: %s", rec.Body.String())
	}
}

// Rute admin tidak boleh ada di mux PUBLIK. Mux inilah yang diteruskan vhost
// nginx ke internet, dan satu rute persetujuan di sini membuat seluruh
// gerbangnya tidak ada artinya.
func TestMuxPublikTidakPunyaRuteAdmin(t *testing.T) {
	d, _ := contoh(t)
	pub := Handler(d)

	for _, p := range []string{"/admin/devices", "/v1/admin/devices", "/admin/devices/x/approve"} {
		rec := minta(t, pub, http.MethodGet, p, "token-daftar", "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, mau 404", p, rec.Code)
		}
	}
	// Rute jamak dari rancangan multi-server juga tidak boleh hidup lagi.
	for _, p := range []string{"/v1/servers", "/v1/servers/s1/status", "/v1/overview", "/v1/status"} {
		if rec := minta(t, pub, http.MethodGet, p, "token-daftar", ""); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, mau 404", p, rec.Code)
		}
	}
}

// Pendaftaran dengan token salah harus ditolak, kalau tidak gerbangnya cuma
// menunda penyerang satu langkah.
func TestDaftarTokenSalahDitolak(t *testing.T) {
	d, store := contoh(t)
	pub := Handler(d)

	if rec := minta(t, pub, http.MethodPost, "/v1/devices", "salah", `{"name":"HP"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("= %d, mau 401", rec.Code)
	}
	if len(store.List()) != 0 {
		t.Error("perangkat tetap tercatat walau token salah")
	}
}

// Config tanpa token pendaftaran menutup pendaftaran, bukan membukanya.
func TestTanpaEnrollTokenPendaftaranDitutup(t *testing.T) {
	d, _ := contoh(t)
	d.EnrollToken = ""

	for _, token := range []string{"", "apa saja"} {
		rec := minta(t, Handler(d), http.MethodPost, "/v1/devices", token, `{"name":"HP"}`)
		if rec.Code == http.StatusCreated {
			t.Fatalf("token kosong meloloskan pendaftaran: %s", rec.Body.String())
		}
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("= %d, mau 503", rec.Code)
		}
	}
}

// /healthz sengaja terbuka: app desktop memakainya untuk membedakan
// "terpasang tapi mati" dari "jalan", termasuk saat tokennya belum diset.
func TestHealthzTerbukaDanTidakMembocorkanApaPun(t *testing.T) {
	d, _ := contoh(t)
	for _, dep := range []Deps{d, {Version: "uji", StartedAt: time.Now()}} {
		rec := minta(t, Handler(dep), http.MethodGet, "/healthz", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /healthz = %d, mau 200", rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("respons bukan JSON: %v", err)
		}
		for _, k := range []string{"enrollToken", "token", "host", "status", "devices"} {
			if _, ada := body[k]; ada {
				t.Errorf("/healthz membocorkan %q: %v", k, body)
			}
		}
	}
}

// Satu permintaan harus cukup untuk menggambar layar HP: identitas mesin,
// metriknya, dan versi agent sekaligus.
func TestServer_SatuResponsMemuatSemuanya(t *testing.T) {
	d, store := contoh(t)
	pub := Handler(d)
	_, token := daftar(t, pub, "HP")
	if _, err := store.SetStatus(store.List()[0].ID, devices.StatusApproved); err != nil {
		t.Fatal(err)
	}

	rec := minta(t, pub, http.MethodGet, "/v1/server", token, "")
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
