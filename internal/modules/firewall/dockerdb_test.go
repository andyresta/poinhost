package firewall

import "testing"

// Regresi untuk laporan "panel Database bilang terblokir padahal aksesnya
// jalan".
//
// Penyebabnya: modul Database dulu mengenali aturan dari KOMENTARnya
// ("poinhost-docker-access"). Aturan yang sah tapi ditulis lewat panel
// Firewall — atau manual oleh user — punya komentar lain, jadi tidak
// dikenali. Sekarang yang dinilai adalah EFEK aturannya, apa pun komentarnya.
func TestCakupanTidakBergantungPadaKomentarAturan(t *testing.T) {
	subnets := []DockerSubnet{{Network: "bridge", Subnet: "172.17.0.0/16"}}

	kasus := []struct {
		nama     string
		komentar string
	}{
		{"ditulis fitur Database", "poinhost-docker-access"},
		{"ditulis panel Firewall", "Docker ke database poinhost-firewall"},
		{"ditulis manual user", "buat docker"},
		{"tanpa komentar sama sekali", ""},
	}
	for _, k := range kasus {
		rules := []Rule{
			{Port: "3306", Protocol: "tcp", Source: "172.16.0.0/12", Action: "allow", Comment: k.komentar},
			{Port: "5432", Protocol: "tcp", Source: "172.16.0.0/12", Action: "allow", Comment: k.komentar},
		}
		salinan := append([]DockerSubnet(nil), subnets...)
		if !markCoverage(salinan, rules) {
			t.Fatalf("%s: aturannya sah, harus terhitung mencakup apa pun komentarnya", k.nama)
		}
		if !subnetAllowedForPort(rules, "172.17.0.0/16", 3306) {
			t.Fatalf("%s: port 3306 seharusnya terhitung diizinkan", k.nama)
		}
	}
}

// Sebaliknya, komentar yang "benar" TIDAK boleh membuat aturan yang tidak
// relevan ikut terhitung — yang menentukan tetap port dan sumbernya.
func TestKomentarYangBenarTidakMembuatAturanSalahTerhitung(t *testing.T) {
	rules := []Rule{
		// Port lain.
		{Port: "8080", Protocol: "tcp", Source: "172.16.0.0/12", Action: "allow", Comment: "poinhost-docker-access"},
		// Sumber di luar rentang Docker.
		{Port: "3306", Protocol: "tcp", Source: "192.168.1.0/24", Action: "allow", Comment: "poinhost-docker-access"},
		// Aturan deny.
		{Port: "3306", Protocol: "tcp", Source: "172.16.0.0/12", Action: "deny", Comment: "poinhost-docker-access"},
	}
	if subnetAllowedForPort(rules, "172.17.0.0/16", 3306) {
		t.Fatal("tidak satu pun aturan di atas benar-benar mengizinkan 3306 dari subnet Docker")
	}
}
