package dockerxfer

import (
	"context"
	"testing"

	"github.com/andyresta/poinhost/internal/modules/docker"
)

func TestMountLabel(t *testing.T) {
	cases := []struct {
		m    docker.MigrationMount
		want string
	}{
		{docker.MigrationMount{HostSide: "/srv/data", ContainerPath: "/data"}, "bind:/srv/data"},
		{docker.MigrationMount{HostSide: "app_data", ContainerPath: "/data"}, "volume:app_data"},
	}
	for _, c := range cases {
		if got := mountLabel(c.m); got != c.want {
			t.Errorf("mountLabel(%+v) = %q, mau %q", c.m, got, c.want)
		}
	}
}

func TestParseStatOutput(t *testing.T) {
	st := parseStatOutput("12\n4096\n")
	if st.files != 12 || st.bytes != 4096 {
		t.Fatalf("parseStatOutput = %+v, mau {12 4096}", st)
	}

	// Output kosong/rusak tidak boleh panic, cukup nol.
	st = parseStatOutput("")
	if st.files != 0 || st.bytes != 0 {
		t.Fatalf("parseStatOutput kosong = %+v, mau nol", st)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{2048, "2.0 KiB"},
		{5 * 1024 * 1024, "5.0 MiB"},
	}
	for _, c := range cases {
		if got := formatBytes(c.n); got != c.want {
			t.Errorf("formatBytes(%d) = %q, mau %q", c.n, got, c.want)
		}
	}
}

// Item image harus selalu jadi entri TERAKHIR di daftar (dipakai job.go
// untuk mengenali item mana yang image saat men-set status akhirnya) —
// SetBlueprint menambah mount dulu baru image, urutan ini harus terjaga.
func TestSetBlueprint_ImageItemIsLast(t *testing.T) {
	j := newJob(context.Background(), StartRequest{}, nil)
	bp := &docker.MigrationBlueprint{
		Name:  "app",
		Image: "nginx:1.25",
		Mounts: []docker.MigrationMount{
			{HostSide: "/srv/data", ContainerPath: "/data"},
			{HostSide: "app_data", ContainerPath: "/var/lib/app"},
		},
	}
	j.SetBlueprint(bp)

	snap := j.Snapshot()
	if len(snap.Items) != 3 {
		t.Fatalf("len(Items) = %d, mau 3", len(snap.Items))
	}
	last := snap.Items[len(snap.Items)-1]
	if last.Label != "image:nginx:1.25" {
		t.Fatalf("item terakhir = %q, mau image:nginx:1.25", last.Label)
	}
	for _, it := range snap.Items {
		if it.Status != ItemPending {
			t.Errorf("status awal item %q = %q, mau pending", it.Label, it.Status)
		}
	}
}
