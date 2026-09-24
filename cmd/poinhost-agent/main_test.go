package main

import (
	"testing"
	"time"

	"github.com/andyresta/poinhost/internal/agent/agentcfg"
	"github.com/andyresta/poinhost/internal/modules/servers"
)

func selfCfg() agentcfg.SelfServer {
	return agentcfg.SelfServer{
		Enabled: true, Name: "This server", Host: "127.0.0.1", Port: 22, Username: "root",
	}
}

func TestIsSelfEntry(t *testing.T) {
	self := selfCfg()

	cocok := &servers.Server{Host: "127.0.0.1", Username: "root", Tags: []string{selfServerTag}}
	if !isSelfEntry(cocok, self) {
		t.Error("entri buatan agent tidak dikenali")
	}

	// Server lain yang kebetulan juga 127.0.0.1 TAPI tidak bertag agent — mis.
	// didaftarkan manual user — tidak boleh ikut dianggap milik agent, karena
	// entri yang dianggap milik agent akan ikut dihapus sebagai duplikat.
	for nama, srv := range map[string]*servers.Server{
		"tanpa tag":     {Host: "127.0.0.1", Username: "root"},
		"user berbeda":  {Host: "127.0.0.1", Username: "deploy", Tags: []string{selfServerTag}},
		"host berbeda":  {Host: "10.0.0.9", Username: "root", Tags: []string{selfServerTag}},
		"tag lain saja": {Host: "127.0.0.1", Username: "root", Tags: []string{"produksi"}},
	} {
		if isSelfEntry(srv, self) {
			t.Errorf("%s: keliru dianggap entri agent", nama)
		}
	}
	if isSelfEntry(nil, self) {
		t.Error("nil dianggap entri agent")
	}
}

// Yang dipertahankan harus entri yang ID-nya sudah tercatat di config, supaya
// pilihannya stabil dan referensi yang sudah ada tidak putus.
func TestPickSelfEntry_MengutamakanIDDariConfig(t *testing.T) {
	now := time.Now()
	mine := []*servers.Server{
		{ID: "baru", CreatedAt: now},
		{ID: "lama", CreatedAt: now.Add(-time.Hour)},
		{ID: "tercatat", CreatedAt: now.Add(-time.Minute)},
	}
	if got := pickSelfEntry(mine, "tercatat"); got.ID != "tercatat" {
		t.Errorf("dipertahankan %q, mau yang tercatat di config", got.ID)
	}
}

// Tanpa ID di config, yang dipertahankan harus yang paling lama dibuat —
// kalau tidak, pilihannya berubah-ubah tiap start dan entri yang dipakai bot
// ikut berganti.
func TestPickSelfEntry_TanpaIDMemilihYangTertua(t *testing.T) {
	now := time.Now()
	mine := []*servers.Server{
		{ID: "b", CreatedAt: now},
		{ID: "a", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "c", CreatedAt: now.Add(-time.Hour)},
	}
	if got := pickSelfEntry(mine, ""); got.ID != "a" {
		t.Errorf("dipertahankan %q, mau yang tertua (a)", got.ID)
	}
	// Dan hasilnya harus sama kalau dipanggil lagi dengan urutan yang berbeda.
	mine2 := []*servers.Server{mine[1], mine[2], mine[0]}
	if got := pickSelfEntry(mine2, ""); got.ID != "a" {
		t.Errorf("hasil berubah karena urutan masukan: %q", got.ID)
	}
}
