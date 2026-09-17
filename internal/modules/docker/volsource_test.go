package docker

import "testing"

// Sisi host sebuah bind boleh berupa path absolut ATAU nama named volume.
// Dulu hanya path absolut yang diterima, sehingga container yang memakai
// named volume — cara yang justru dianjurkan untuk data yang harus bertahan —
// gagal dibuat ulang dengan pesan "path host volume harus absolut", padahal
// konfigurasinya sah menurut Docker.
func TestIsVolumeSource(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		// Bind mount biasa.
		{"/var/www/html", true},
		{"/", true},
		// Named volume, termasuk bentuk yang dihasilkan docker compose
		// (nama proyek + garis bawah + nama volume).
		{"devpoin-app_data", true},
		{"pgdata", true},
		{"redis.data-1", true},
		{"A1", true},
		// Path relatif: artinya bergantung direktori kerja sesi SSH, yang
		// tidak terlihat maupun dikendalikan user.
		{"./data", false},
		{"../data", false},
		{"data/sub", false},
		// Nama yang tidak sah sebagai volume Docker.
		{"-awalanstrip", false},
		{"_awalangaris", false},
		{"ada spasi", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isVolumeSource(c.host); got != c.want {
			t.Errorf("isVolumeSource(%q) = %v, mau %v", c.host, got, c.want)
		}
	}
}

// Named volume harus lolos validasi recreate secara utuh, bukan cuma lolos
// pemeriksaan sumbernya.
func TestValidateRecreateOverrides_AcceptsNamedVolume(t *testing.T) {
	err := validateRecreateOverrides(
		nil, nil,
		[]VolumeMount{{HostPath: "devpoin-app_data", ContainerPath: "/var/lib/app"}},
		0, nil,
	)
	if err != nil {
		t.Fatalf("named volume seharusnya diterima, dapat error: %v", err)
	}
}

// Sisi container tetap wajib absolut — Docker memang tidak menerima yang lain.
func TestValidateRecreateOverrides_RejectsRelativeContainerPath(t *testing.T) {
	err := validateRecreateOverrides(
		nil, nil,
		[]VolumeMount{{HostPath: "/srv/data", ContainerPath: "data"}},
		0, nil,
	)
	if err == nil {
		t.Fatal("path container relatif seharusnya ditolak")
	}
}
