package dockerxfer

import (
	"context"
	"strings"
	"testing"

	"github.com/andyresta/poinhost/internal/modules/docker"
)

func composeBlueprint(workdir string, mounts ...docker.MigrationMount) *docker.MigrationBlueprint {
	return &docker.MigrationBlueprint{
		Name:   "web",
		Image:  "proyek-web",
		Mounts: mounts,
		Labels: map[string]string{
			docker.LabelComposeProject:    "proyek",
			docker.LabelComposeWorkingDir: workdir,
		},
	}
}

func TestBuildPlan_WithoutWorkdirKeepsMounts(t *testing.T) {
	bp := composeBlueprint("/home/andy/proyek", docker.MigrationMount{HostSide: "/home/andy/proyek/data"})
	plan, err := buildPlan(bp, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Workdir != "" || len(plan.Mounts) != 1 || len(plan.Covered) != 0 {
		t.Fatalf("plan = %+v, mau mount apa adanya tanpa workdir", plan)
	}
}

// Bind mount relatif di compose (./data) sudah di-resolve jadi path absolut
// di dalam workdir — tidak boleh di-tar dua kali.
func TestBuildPlan_DedupesMountsInsideWorkdir(t *testing.T) {
	bp := composeBlueprint("/home/andy/proyek/",
		docker.MigrationMount{HostSide: "/home/andy/proyek/data", ContainerPath: "/app/data"},
		docker.MigrationMount{HostSide: "/home/andy/proyek", ContainerPath: "/app"},
		docker.MigrationMount{HostSide: "/home/andy/proyek-lain/x", ContainerPath: "/x"},
		docker.MigrationMount{HostSide: "/srv/uploads", ContainerPath: "/uploads"},
		docker.MigrationMount{HostSide: "pgdata", ContainerPath: "/var/lib/postgresql/data"},
	)
	plan, err := buildPlan(bp, true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Workdir != "/home/andy/proyek" {
		t.Fatalf("Workdir = %q, mau /home/andy/proyek (dibersihkan)", plan.Workdir)
	}
	if len(plan.Covered) != 2 {
		t.Fatalf("Covered = %+v, mau 2 mount di dalam workdir", plan.Covered)
	}
	var got []string
	for _, it := range plan.items() {
		got = append(got, it.key)
	}
	want := []string{
		"workdir:/home/andy/proyek",
		"bind:/home/andy/proyek-lain/x", // prefix nama mirip, BUKAN di dalam workdir
		"bind:/srv/uploads",
		"volume:pgdata",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("items = %v, mau %v", got, want)
	}
}

func TestBuildPlan_RejectsNonComposeAndDangerousDirs(t *testing.T) {
	bp := &docker.MigrationBlueprint{Name: "plain", Labels: map[string]string{}}
	if _, err := buildPlan(bp, true); err == nil {
		t.Fatal("container non-compose harus ditolak saat CopyWorkdir")
	}
	for _, dir := range []string{"/", "/etc", "/home", "/var/", "relatif/path"} {
		if _, err := buildPlan(composeBlueprint(dir), true); err == nil {
			t.Errorf("workdir %q harus ditolak", dir)
		}
	}
	for _, dir := range []string{"/root", "/home/andy", "/opt/app", "/srv/web"} {
		if _, err := buildPlan(composeBlueprint(dir), true); err != nil {
			t.Errorf("workdir %q harus diterima: %v", dir, err)
		}
	}
}

func TestSetPlan_WorkdirItemFirstWithCoveredNote(t *testing.T) {
	bp := composeBlueprint("/home/andy/proyek",
		docker.MigrationMount{HostSide: "/home/andy/proyek/data"},
		docker.MigrationMount{HostSide: "pgdata"},
	)
	plan, err := buildPlan(bp, true)
	if err != nil {
		t.Fatal(err)
	}
	j := newJob(context.Background(), StartRequest{CopyWorkdir: true}, nil)
	j.SetPlan(bp, plan)

	snap := j.Snapshot()
	if !snap.CopyWorkdir {
		t.Error("Progress.CopyWorkdir harus true")
	}
	if len(snap.Items) != 3 {
		t.Fatalf("len(Items) = %d, mau 3 (workdir, volume, image)", len(snap.Items))
	}
	first := snap.Items[0]
	if first.Label != "workdir:/home/andy/proyek" {
		t.Fatalf("item pertama = %q", first.Label)
	}
	if !strings.Contains(first.Warning, "/home/andy/proyek/data") {
		t.Errorf("warning workdir = %q, mau menyebut bind mount yang tercakup", first.Warning)
	}
	if snap.Items[2].Label != "image:proyek-web" {
		t.Errorf("item terakhir = %q, mau image", snap.Items[2].Label)
	}
}
