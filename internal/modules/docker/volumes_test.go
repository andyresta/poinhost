package docker

import "testing"

// Named volume harus muncul di ContainerInspectResponse.Volumes, bukan
// diam-diam hilang. Sebelumnya parseVolumes hanya menyerap mount bertipe
// "bind" — container yang datanya disimpan di named volume (cara yang
// dianjurkan Docker) tampil seolah tidak punya volume sama sekali, dan
// recreate/migrasi jadi diam-diam menjatuhkan data itu.
func TestInspectToDetail_IncludesNamedVolume(t *testing.T) {
	raw := `{
	  "Id": "abc123",
	  "Name": "/app",
	  "Config": {"Image": "app:latest"},
	  "HostConfig": {"NetworkMode": "bridge"},
	  "State": {"Status": "running"},
	  "Mounts": [
	    {"Type": "volume", "Name": "app_data", "Source": "/var/lib/docker/volumes/app_data/_data", "Destination": "/var/lib/app", "RW": true},
	    {"Type": "bind", "Source": "/srv/config", "Destination": "/etc/app", "RW": false}
	  ]
	}`
	inv, err := parseInspectJSON(raw)
	if err != nil {
		t.Fatalf("parseInspectJSON: %v", err)
	}
	detail, tpl, err := inspectToDetail(inv)
	if err != nil {
		t.Fatalf("inspectToDetail: %v", err)
	}
	if len(detail.Volumes) != 2 {
		t.Fatalf("Volumes = %v, mau 2 entri", detail.Volumes)
	}

	var named, bind *VolumeMount
	for i := range detail.Volumes {
		switch detail.Volumes[i].HostPath {
		case "app_data":
			named = &detail.Volumes[i]
		case "/srv/config":
			bind = &detail.Volumes[i]
		}
	}
	if named == nil {
		t.Fatalf("named volume 'app_data' tidak ditemukan: %v", detail.Volumes)
	}
	if named.ContainerPath != "/var/lib/app" || named.ReadOnly {
		t.Errorf("named volume salah: %+v", named)
	}
	if !IsNamedVolume(named.HostPath) {
		t.Errorf("IsNamedVolume(%q) = false, mau true", named.HostPath)
	}

	if bind == nil {
		t.Fatalf("bind mount '/srv/config' tidak ditemukan: %v", detail.Volumes)
	}
	if bind.ContainerPath != "/etc/app" || !bind.ReadOnly {
		t.Errorf("bind mount salah: %+v", bind)
	}
	if IsNamedVolume(bind.HostPath) {
		t.Errorf("IsNamedVolume(%q) = true, mau false", bind.HostPath)
	}

	// Template create (dipakai recreate) harus membawa volume yang sama,
	// termasuk yang named volume — bukan cuma respons UI.
	if len(tpl.Volumes) != 2 {
		t.Fatalf("tpl.Volumes = %v, mau 2 entri", tpl.Volumes)
	}
}

func TestIsNamedVolume(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"app_data", true},
		{"devpoin-app_data", true},
		{"/srv/data", false},
		{"/", false},
		{"", false},
		{"./data", false},
	}
	for _, c := range cases {
		if got := IsNamedVolume(c.host); got != c.want {
			t.Errorf("IsNamedVolume(%q) = %v, mau %v", c.host, got, c.want)
		}
	}
}
