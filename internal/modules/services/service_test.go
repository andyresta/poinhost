package services

import "testing"

// Keluaran systemctl yang ditiru di sini sengaja mencakup kasus yang
// gampang salah baca: deskripsi berisi spasi, unit yang PUNYA file tapi
// tidak pernah di-load (tidak muncul di list-units), unit static/masked
// yang tidak boleh ditawari tombol enable, dan unit template "@." yang
// tidak bisa dioperasikan langsung.
const sampleStdout = "__UNIT_FILES__\n" +
	"nginx.service\x1fenabled\n" +
	"mariadb.service\x1fdisabled\n" +
	"cron.service\x1fstatic\n" +
	"never-run.service\x1fdisabled\n" +
	"getty@.service\x1fenabled\n" +
	"blocked.service\x1fmasked\n" +
	"__UNITS__\n" +
	"nginx.service\x1factive\x1frunning\x1fA high performance web server\n" +
	"mariadb.service\x1finactive\x1fdead\x1fMariaDB 10.11 database server\n" +
	"cron.service\x1factive\x1frunning\x1fRegular background program processing daemon\n" +
	"blocked.service\x1finactive\x1fdead\x1fSome masked unit\n"

func TestParseListJoinsUnitFilesWithRunningUnits(t *testing.T) {
	out := parseList(sampleStdout)
	if !out.SystemdAvailable {
		t.Fatal("systemd seharusnya terdeteksi ada")
	}

	byName := map[string]ServiceInfo{}
	for _, s := range out.Services {
		byName[s.Name] = s
	}

	// Sufiks ".service" dibuang untuk tampilan.
	nginx, ok := byName["nginx"]
	if !ok {
		t.Fatalf("nginx tidak ada di hasil: %+v", out.Services)
	}
	if !nginx.Running || !nginx.Enabled {
		t.Fatalf("nginx seharusnya running+enabled, got %+v", nginx)
	}
	if nginx.Description != "A high performance web server" {
		t.Fatalf("deskripsi berspasi terpotong: %q", nginx.Description)
	}

	// Dua dimensi yang BERBEDA: mariadb terpasang autostart-nya mati dan
	// memang sedang tidak jalan.
	maria := byName["mariadb"]
	if maria.Running || maria.Enabled {
		t.Fatalf("mariadb seharusnya mati dan tidak autostart, got %+v", maria)
	}
	if !maria.CanEnable {
		t.Fatal("unit disabled seharusnya masih bisa di-enable")
	}

	// Unit static: sedang jalan, tapi enable/disable tidak berlaku.
	cron := byName["cron"]
	if !cron.Running {
		t.Fatalf("cron seharusnya running, got %+v", cron)
	}
	if cron.CanEnable {
		t.Fatal("unit static tidak boleh ditawari enable/disable")
	}
	if byName["blocked"].CanEnable {
		t.Fatal("unit masked tidak boleh ditawari enable/disable")
	}

	// Punya unit file tapi tidak pernah di-load: harus tetap muncul (justru
	// inilah yang biasanya ingin di-enable), dilaporkan inactive — bukan
	// hilang dari daftar dan bukan dianggap failed.
	never, ok := byName["never-run"]
	if !ok {
		t.Fatal("unit yang belum pernah jalan seharusnya tetap terdaftar")
	}
	if never.Running || never.ActiveState != "inactive" {
		t.Fatalf("unit belum pernah jalan seharusnya inactive, got %+v", never)
	}

	// Template unit tidak bisa dioperasikan langsung — harus disaring.
	if _, ok := byName["getty@"]; ok {
		t.Fatal("unit template (@.) seharusnya tidak masuk daftar")
	}
}

func TestParseListDetectsMissingSystemd(t *testing.T) {
	out := parseList("__NO_SYSTEMD__\n")
	if out.SystemdAvailable {
		t.Fatal("seharusnya melaporkan systemd tidak tersedia")
	}
	if len(out.Services) != 0 {
		t.Fatalf("tidak boleh ada service, got %d", len(out.Services))
	}
}

// Nama unit ikut masuk ke perintah shell, jadi validasinya harus menolak
// apa pun di luar karakter yang memang dipakai systemd.
func TestNormalizeUnitName(t *testing.T) {
	got, err := normalizeUnitName("nginx")
	if err != nil || got != "nginx.service" {
		t.Fatalf("normalizeUnitName(nginx) = %q, %v", got, err)
	}
	if got, _ := normalizeUnitName("mariadb.service"); got != "mariadb.service" {
		t.Fatalf("sufiks yang sudah ada tidak boleh digandakan: %q", got)
	}
	for _, bad := range []string{"", "   ", "nginx; rm -rf /", "a b", "foo$(id)", "`id`"} {
		if _, err := normalizeUnitName(bad); err == nil {
			t.Fatalf("nama %q seharusnya ditolak", bad)
		}
	}
}

func TestServiceActionRejectsUnknownAction(t *testing.T) {
	svc := &Service{}
	if _, err := svc.ServiceAction("srv", "nginx", "rm -rf"); err == nil {
		t.Fatal("aksi di luar daftar seharusnya ditolak sebelum menyentuh SSH")
	}
}
