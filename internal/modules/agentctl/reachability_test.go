package agentctl

import "testing"

// Panel mengenali domainnya dengan mencocokkan target proxy PERSIS. Kalau
// bentuk target yang dicari berbeda satu karakter saja dari yang ditulis ke
// vhost, domain yang sudah benar-benar mem-proxy ke agent akan dilaporkan
// "belum bisa dijangkau dari luar".
func TestAgentProxyTarget_BentuknyaPersis(t *testing.T) {
	if got := agentProxyTarget(7898); got != "http://127.0.0.1:7898" {
		t.Errorf("target = %q", got)
	}
	// Port lain bukan milik agent ini: satu mesin bisa menjalankan hal lain
	// di loopback, dan mengakuinya sebagai agent akan menampilkan URL yang
	// menuju aplikasi orang lain.
	if agentProxyTarget(7898) == agentProxyTarget(7899) {
		t.Error("port berbeda menghasilkan target yang sama")
	}
}

func TestPublishedURL(t *testing.T) {
	if got := publishedURL("agent.example.com", true); got != "https://agent.example.com" {
		t.Errorf("dengan ssl = %q", got)
	}
	// Tanpa sertifikat, URL-nya http — menampilkan https di sini akan
	// mengirim HP ke alamat yang tidak menjawab.
	if got := publishedURL("agent.example.com", false); got != "http://agent.example.com" {
		t.Errorf("tanpa ssl = %q", got)
	}
}
