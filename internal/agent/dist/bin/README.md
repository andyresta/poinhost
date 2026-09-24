# Binary agent yang di-embed

Folder ini menampung binary `poinhost-agent` terkompresi yang ikut dibundel ke
dalam app desktop, lalu dikirim ke server lewat SFTP saat user menekan
**Pasang** di menu Agent Control. Instalasi karena itu tidak pernah mengunduh
apa pun dari internet — penting karena server target sering berada di balik
proxy atau firewall keluar yang ketat.

Isinya dibuat oleh CI saat rilis (lihat `.github/workflows/release.yml`):

    poinhost-agent-linux-amd64.gz
    poinhost-agent-linux-arm64.gz

Keduanya artefak build, jadi tidak di-commit (lihat `.gitignore`). File
README.md ini justru **wajib ada dan di-commit**: pola `//go:embed` gagal saat
kompilasi kalau tidak cocok dengan satu file pun, sehingga tanpa file ini
`go build` akan patah di clone yang bersih — padahal binary agent memang belum
dibangun di sana.

Membangunnya secara lokal:

    make agent

atau manual:

    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=$VERSION" \
        -o /tmp/poinhost-agent ./cmd/poinhost-agent
    gzip -9 -c /tmp/poinhost-agent > internal/agent/dist/poinhost-agent-linux-amd64.gz

Tanpa file-file itu app desktop tetap dibangun dan berjalan normal; hanya
tombol Pasang di Agent Control yang nonaktif, dengan keterangan bahwa build ini
tidak membawa binary agent.
