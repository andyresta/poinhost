package docker

import (
	"strings"
	"testing"
)

// blueprintToTemplate harus membawa SEMUA mount (bind maupun named volume)
// apa adanya ke createTemplate — inilah satu-satunya titik dipakai
// CreateFromBlueprint, jadi kalau ada field yang hilang di sini, container
// hasil migrasi akan diam-diam kehilangan konfigurasi itu.
func TestBlueprintToTemplate_KeepsMountsAndNetwork(t *testing.T) {
	bp := MigrationBlueprint{
		Name:  "app",
		Image: "app:latest",
		Mounts: []MigrationMount{
			{HostSide: "/srv/data", ContainerPath: "/data", ReadOnly: true},
			{HostSide: "app_cache", ContainerPath: "/cache"},
		},
		PrimaryNetwork: "custom-net",
		PrimaryAliases: []string{"app"},
		ExtraNetworks: []MigrationNetworkAttachment{
			{Name: "backend-net", Aliases: []string{"app-backend"}},
		},
	}
	tpl := blueprintToTemplate(bp)

	if len(tpl.Volumes) != 2 {
		t.Fatalf("tpl.Volumes = %v, mau 2 entri", tpl.Volumes)
	}
	if tpl.Volumes[0].HostPath != "/srv/data" || !tpl.Volumes[0].ReadOnly {
		t.Errorf("bind mount salah: %+v", tpl.Volumes[0])
	}
	if tpl.Volumes[1].HostPath != "app_cache" {
		t.Errorf("named volume salah: %+v", tpl.Volumes[1])
	}
	if tpl.NetworkMode != "custom-net" {
		t.Errorf("NetworkMode = %q, mau custom-net", tpl.NetworkMode)
	}
	if len(tpl.NetworkAliases) != 1 || tpl.NetworkAliases[0] != "app" {
		t.Errorf("NetworkAliases = %v, mau [app]", tpl.NetworkAliases)
	}

	// buildCreateArgs harus tetap berhasil dari blueprint yang sudah
	// dikonversi — kalau ini gagal, CreateFromBlueprint akan gagal untuk
	// SEMUA container yang punya mount, bukan cuma kasus tepi.
	args, err := buildCreateArgs(tpl)
	if err != nil {
		t.Fatalf("buildCreateArgs: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-v /srv/data:/data:ro") {
		t.Errorf("bind mount tidak muncul di args: %s", joined)
	}
	if !strings.Contains(joined, "-v app_cache:/cache") {
		t.Errorf("named volume tidak muncul di args: %s", joined)
	}
}

// Network TAMBAHAN (di luar network utama) harus tercatat di blueprint,
// bukan hilang begitu saja — inilah perbaikan atas homepoin, yang cuma
// membawa satu network (NetworkMode) dan menjatuhkan semua yang lain tanpa
// peringatan.
func TestInspectMigrationBlueprint_CapturesExtraNetworks(t *testing.T) {
	raw := `{
	  "Id": "abc123def456",
	  "Name": "/app",
	  "Config": {"Image": "app:latest"},
	  "HostConfig": {"NetworkMode": "backend"},
	  "State": {"Status": "running"},
	  "NetworkSettings": {
	    "Networks": {
	      "backend": {"Aliases": ["app", "abc123def456"]},
	      "frontend": {"Aliases": ["app-web"]}
	    }
	  }
	}`
	inv, err := parseInspectJSON(raw)
	if err != nil {
		t.Fatalf("parseInspectJSON: %v", err)
	}
	_, tpl, err := inspectToDetail(inv)
	if err != nil {
		t.Fatalf("inspectToDetail: %v", err)
	}

	all := attachedNetworks(inv)
	if len(all) != 2 {
		t.Fatalf("attachedNetworks = %v, mau 2 entri", all)
	}

	var extra []MigrationNetworkAttachment
	for name, aliases := range all {
		if name == tpl.NetworkMode {
			continue
		}
		extra = append(extra, MigrationNetworkAttachment{Name: name, Aliases: aliases})
	}
	if len(extra) != 1 || extra[0].Name != "frontend" {
		t.Fatalf("ExtraNetworks = %v, mau [{frontend [app-web]}]", extra)
	}
	if len(extra[0].Aliases) != 1 || extra[0].Aliases[0] != "app-web" {
		t.Errorf("alias network tambahan = %v, mau [app-web]", extra[0].Aliases)
	}
}

func TestParseComposeMigrationInfo(t *testing.T) {
	out := "MANAGED=1\nPROJECT=proyek\nSERVICE=web\nWORKDIR=/home/andy/proyek\n" +
		"FILES=/home/andy/proyek/docker-compose.yml, /home/andy/proyek/override.yml\n" +
		"EXISTS=1\nBYTES=123456\nSIBLING=proyek-db-1\nSIBLING=proyek-redis-1\n"
	info := parseComposeMigrationInfo(out)
	if !info.Managed || info.Project != "proyek" || info.Service != "web" {
		t.Fatalf("info = %+v", info)
	}
	if info.WorkingDir != "/home/andy/proyek" || !info.WorkingDirExists || info.WorkingDirBytes != 123456 {
		t.Fatalf("workdir info = %+v", info)
	}
	if len(info.ConfigFiles) != 2 || info.ConfigFiles[1] != "/home/andy/proyek/override.yml" {
		t.Fatalf("ConfigFiles = %v", info.ConfigFiles)
	}
	if len(info.Siblings) != 2 || info.Siblings[0] != "proyek-db-1" {
		t.Fatalf("Siblings = %v", info.Siblings)
	}

	plain := parseComposeMigrationInfo("MANAGED=0\n")
	if plain.Managed || plain.Siblings == nil || plain.ConfigFiles == nil {
		t.Fatalf("non-compose = %+v, mau Managed=false dengan slice kosong (bukan nil) untuk JSON", plain)
	}
}
