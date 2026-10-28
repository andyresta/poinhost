package api

import (
	"errors"
	"net/http"

	"github.com/andyresta/poinhost/internal/agent/devices"
)

// AdminDeps adalah yang dibutuhkan permukaan admin.
type AdminDeps struct {
	Devices AdminDeviceStore
}

// AdminDeviceStore adalah bagian devices.Store yang menyetujui dan mencabut.
type AdminDeviceStore interface {
	List() []devices.Device
	SetStatus(id, status string) (devices.Device, error)
	SetControl(id string, allowed bool) (devices.Device, error)
	Delete(id string) error
}

// AdminHandler membangun mux persetujuan perangkat.
//
// Mux ini TIDAK PERNAH dipasang di listener TCP. Ia dilayani lewat unix
// socket di dalam direktori data agent (mode 0700, milik root), jadi yang
// bisa menyentuhnya hanya root di mesin itu — dan app desktop poinhost
// memanggilnya lewat `curl --unix-socket` di dalam sesi SSH-nya.
//
// Karena itu di sini tidak ada token sama sekali, dan itu bukan kelalaian:
// siapa pun yang bisa membuka socket ini sudah root, dan root bisa membaca
// seluruh config agent beserta kuncinya. Token tambahan hanya akan menjadi
// rahasia keempat yang harus disimpan, diputar, dan bisa bocor, tanpa
// menghalangi siapa pun yang belum terhalang izin berkas.
//
// Yang DIHINDARI dengan memisahkan socket ini dari port publik: vhost nginx
// yang menerbitkan agent meneruskan seluruh path ke port agent. Kalau
// persetujuan hidup di port yang sama, ia ikut terbit ke internet dan hanya
// dijaga satu string rahasia lagi — sementara di socket, tidak ada permintaan
// dari internet yang bisa sampai ke sini sama sekali.
func AdminHandler(d AdminDeps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /admin/devices", func(w http.ResponseWriter, r *http.Request) {
		list := d.Devices.List()
		if list == nil {
			list = []devices.Device{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"devices": sanitize(list)})
	})

	mux.HandleFunc("POST /admin/devices/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		d.setStatus(w, r.PathValue("id"), devices.StatusApproved)
	})

	mux.HandleFunc("POST /admin/devices/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
		d.setStatus(w, r.PathValue("id"), devices.StatusRevoked)
	})

	// Izin kendali terpisah dari persetujuan, dan tombolnya pun terpisah:
	// menyetujui HP supaya bisa memantau adalah keputusan yang berbeda dari
	// mengizinkannya menghentikan layanan.
	mux.HandleFunc("POST /admin/devices/{id}/control", func(w http.ResponseWriter, r *http.Request) {
		d.setControl(w, r.PathValue("id"), true)
	})

	mux.HandleFunc("POST /admin/devices/{id}/readonly", func(w http.ResponseWriter, r *http.Request) {
		d.setControl(w, r.PathValue("id"), false)
	})

	mux.HandleFunc("DELETE /admin/devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := d.Devices.Delete(r.PathValue("id")); err != nil {
			writeErr(w, kode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"devices": sanitize(d.Devices.List())})
	})

	return mux
}

func (d AdminDeps) setStatus(w http.ResponseWriter, id, status string) {
	if _, err := d.Devices.SetStatus(id, status); err != nil {
		writeErr(w, kode(err), err.Error())
		return
	}
	// Seluruh daftar dikembalikan, bukan satu perangkat yang berubah: panel
	// desktop menggambar ulang dari jawaban ini, jadi daftar utuh menutup
	// kemungkinan layar menampilkan keadaan setengah lama.
	writeJSON(w, http.StatusOK, map[string]any{"devices": sanitize(d.Devices.List())})
}

func (d AdminDeps) setControl(w http.ResponseWriter, id string, allowed bool) {
	if _, err := d.Devices.SetControl(id, allowed); err != nil {
		writeErr(w, kode(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": sanitize(d.Devices.List())})
}

// sanitize membuang TokenHash sebelum daftar keluar dari agent.
//
// Hash memang tidak bisa dipakai masuk, tapi ia tetap tidak punya urusan di
// layar desktop maupun di log mana pun yang merekam respons ini.
func sanitize(list []devices.Device) []devices.Device {
	out := make([]devices.Device, 0, len(list))
	for _, d := range list {
		d.TokenHash = ""
		out = append(out, d)
	}
	return out
}

func kode(err error) int {
	if errors.Is(err, devices.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
