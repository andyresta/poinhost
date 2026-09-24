package agentctl

import "testing"

func TestParseVersion(t *testing.T) {
	ok := map[string][3]int{
		"1.2.3":   {1, 2, 3},
		"v1.2.3":  {1, 2, 3},
		"0.1.0":   {0, 1, 0},
		"v2.0":    {2, 0, 0},
		"3":       {3, 0, 0},
		"1.2.3+b": {1, 2, 3},
	}
	for in, want := range ok {
		got, valid := parseVersion(in)
		if !valid || got.parts != want {
			t.Errorf("parseVersion(%q) = (%v,%v), mau %v", in, got.parts, valid, want)
		}
	}

	// "dev" adalah yang memicu bug aslinya: build desktop yang belum dirilis
	// ditawarkan sebagai versi tujuan "update".
	for _, in := range []string{"dev", "", "abc", "1.2.3.4", "v-1.0", "1.x.3"} {
		if _, valid := parseVersion(in); valid {
			t.Errorf("parseVersion(%q) dianggap versi rilis", in)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.4", "1.2.3", 1},
		{"1.3.0", "1.2.9", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.2.3", "1.2.4", -1},
		{"v1.0.0", "1.0.0", 0},
		// Pra-rilis lebih tua dari rilis final dengan angka yang sama.
		{"1.0.0", "1.0.0-rc1", 1},
		{"1.0.0-rc1", "1.0.0", -1},
		{"1.0.0-rc2", "1.0.0-rc1", 1},
	}
	for _, c := range cases {
		av, _ := parseVersion(c.a)
		bv, _ := parseVersion(c.b)
		if got := compareVersions(av, bv); got != c.want {
			t.Errorf("compare(%q,%q) = %d, mau %d", c.a, c.b, got, c.want)
		}
	}
}

// Bug yang dilaporkan user: build dev menawarkan "Update to dev" terhadap
// agent 0.1.0 yang terpasang. "dev" bukan versi, jadi urutannya tidak
// diketahui — dan yang tidak diketahui tidak ditawarkan sama sekali.
func TestDecideVersionAction_VersiTidakBisaDiurutkanTidakDitawarkan(t *testing.T) {
	for _, c := range []struct{ installed, bundled string }{
		{"0.1.0", "dev"},
		{"dev", "0.1.0"},
		{"dev", "dev"},
		{"0.1.0", ""},
		{"", "0.1.0"},
		{"0.1.0", "abc"},
	} {
		if got := decideVersionAction(c.installed, c.bundled); got != versionNothing {
			t.Errorf("decideVersionAction(%q,%q) = %v, mau versionNothing", c.installed, c.bundled, got)
		}
	}
}

// Sisi lain dari bug yang sama: perbandingan `!=` juga menawarkan PENURUNAN
// versi sebagai update, mis. app desktop lama dipakai terhadap agent baru.
func TestDecideVersionAction_TidakMenawarkanPenurunanVersi(t *testing.T) {
	if got := decideVersionAction("0.9.0", "0.2.0"); got != versionNothing {
		t.Errorf("bundled lebih tua = %v, mau versionNothing", got)
	}
	if got := decideVersionAction("v1.0.0", "v1.0.0-rc1"); got != versionNothing {
		t.Errorf("bundled pra-rilis terhadap rilis final = %v, mau versionNothing", got)
	}
}

func TestDecideVersionAction_UpdateNyata(t *testing.T) {
	for _, c := range []struct{ installed, bundled string }{
		{"0.1.0", "0.2.0"},
		{"v0.1.0", "v1.0.0"},
		{"1.2.3", "1.2.4"},
		{"1.0.0-rc1", "1.0.0"},
	} {
		if got := decideVersionAction(c.installed, c.bundled); got != versionUpdate {
			t.Errorf("decideVersionAction(%q,%q) = %v, mau versionUpdate", c.installed, c.bundled, got)
		}
	}
}

func TestDecideVersionAction_VersiSamaTidakMenawarkanApaPun(t *testing.T) {
	for _, c := range []struct{ installed, bundled string }{
		{"0.1.0", "0.1.0"},
		{" 0.1.0 ", "0.1.0"},
		{"v0.1.0", "0.1.0"},
	} {
		if got := decideVersionAction(c.installed, c.bundled); got != versionNothing {
			t.Errorf("decideVersionAction(%q,%q) = %v, mau versionNothing", c.installed, c.bundled, got)
		}
	}
}
