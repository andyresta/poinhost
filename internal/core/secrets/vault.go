// Package secrets menyimpan string rahasia (password database, dst) di
// mesin LOKAL user — TIDAK PERNAH ditulis ke server target yang dikelola.
//
// Ini beda mendasar dari homepoin: fitur "MySQL Manager"-nya menaruh file
// JSON terenkripsi (kunci diturunkan dari satu master password level-
// aplikasi) DI SERVER TARGET itu sendiri (/root/.homepoin_mysql_creds.json),
// supaya semua instance/operator homepoin yang mengelola server yang sama
// bisa memakainya. poinhost adalah aplikasi desktop single-user — tidak ada
// alasan menaruh artefak kredensial tambahan di server produksi customer,
// jadi rahasia disimpan lokal saja di mesin yang menjalankan poinhost.
package secrets

// Vault menyimpan/mengambil/menghapus satu string rahasia berdasarkan key.
// Get mengembalikan ok=false (tanpa error) kalau key belum pernah disimpan.
type Vault interface {
	Get(key string) (value string, ok bool, err error)
	Set(key, value string) error
	Delete(key string) error
}
