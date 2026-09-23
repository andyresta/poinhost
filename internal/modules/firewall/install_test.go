package firewall

import (
	"strings"
	"testing"
)

func TestParseInstallInfo(t *testing.T) {
	cases := []struct {
		name, out, rec string
		opts           []string
		canInstall     bool
		blocker        bool
	}{
		{"ubuntu", "ID=ubuntu\nPRETTY=Ubuntu 24.04.4 LTS\nPM=apt\n", "ufw", []string{"ufw", "firewalld"}, true, false},
		{"rocky (dnf+yum)", "ID=rocky\nPRETTY=Rocky Linux 9\nPM=dnf\nPM=yum\n", "firewalld", []string{"firewalld"}, true, false},
		{"opensuse", "ID=opensuse-leap\nPRETTY=openSUSE Leap 15.6\nPM=zypper\n", "firewalld", []string{"firewalld"}, true, false},
		{"arch", "ID=arch\nPRETTY=Arch Linux\nPM=pacman\n", "ufw", []string{"ufw", "firewalld"}, true, false},
		{"alpine", "ID=alpine\nPRETTY=Alpine Linux v3.20\nPM=apk\n", "ufw", []string{"ufw"}, true, false},
		{"tidak dikenal", "ID=weird\nPRETTY=Weird OS\n", "", nil, false, false},
		{"ufw sudah aktif", "ID=ubuntu\nPM=apt\nINSTALLED=ufw\nACTIVE=ufw\n", "ufw", []string{"ufw", "firewalld"}, false, true},
	}
	for _, c := range cases {
		info := parseInstallInfo(c.out, 2222)
		if info.Recommended != c.rec || strings.Join(info.Options, ",") != strings.Join(c.opts, ",") ||
			info.CanInstall != c.canInstall || (info.Blocker != "") != c.blocker || info.SSHPort != 2222 {
			t.Errorf("%s: %+v", c.name, info)
		}
	}
	if info := parseInstallInfo("ID=debian\nPM=apt\nDOCKER=1\n", 22); !info.HasDocker {
		t.Error("DOCKER=1 harus terbaca")
	}
}

// Instalasi TIDAK boleh menyalakan firewall, dan port SSH (termasuk port
// non-standar) harus sudah diizinkan sebelum firewall dinyalakan kapan pun.
func TestInstallFirewallScriptNeverEnables(t *testing.T) {
	combos := [][2]string{
		{"apt", "ufw"}, {"apt", "firewalld"}, {"dnf", "firewalld"}, {"yum", "firewalld"},
		{"zypper", "firewalld"}, {"pacman", "ufw"}, {"pacman", "firewalld"}, {"apk", "ufw"},
	}
	for _, c := range combos {
		s, ok := installFirewallScript(c[0], c[1], 2222)
		if !ok {
			t.Fatalf("%v tidak didukung", c)
		}
		for _, bad := range []string{"ufw --force enable", "ufw enable", "systemctl enable --now firewalld", "systemctl start firewalld"} {
			if strings.Contains(s, bad) {
				t.Errorf("%v menyalakan firewall (%q):\n%s", c, bad, s)
			}
		}
		want := "ufw allow to any port 2222 proto tcp"
		if c[1] == "firewalld" {
			want = "firewall-offline-cmd --add-port=2222/tcp"
			if !strings.Contains(s, "systemctl disable --now firewalld") {
				t.Errorf("%v tidak men-disable firewalld sesudah instal", c)
			}
		}
		if !strings.Contains(s, want) {
			t.Errorf("%v tidak mengizinkan SSH lebih dulu (%q)", c, want)
		}
	}
	s, _ := installFirewallScript("apt", "firewalld", 22)
	if !strings.Contains(s, "/usr/sbin/policy-rc.d") || !strings.Contains(s, "exit 101") {
		t.Error("apt/firewalld harus mencegah postinst menyalakan service lewat policy-rc.d")
	}
	if _, ok := installFirewallScript("dnf", "ufw", 22); ok {
		t.Error("ufw di dnf (butuh EPEL) tidak boleh ditawarkan")
	}
}
