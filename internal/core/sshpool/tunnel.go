package sshpool

import (
	"context"
	"net"
	"time"
)

// DialTunnel membuka koneksi TCP yang ditunnel lewat channel SSH
// ("direct-tcpip") ke address SISI REMOTE server — dipakai untuk membuka
// koneksi protokol asli (mis. MySQL) tanpa exec CLI dan TANPA membuka
// koneksi/handshake SSH baru: memakai slot shared yang SAMA dipakai Exec
// untuk server ini (lihat Pool.tryAcquire), yang sudah dimultiplex jadi
// aman dipanggil bersamaan dari banyak goroutine/tab.
//
// Channel yang dikembalikan dibungkus supaya SetDeadline/SetReadDeadline/
// SetWriteDeadline tidak error (driver seperti go-sql-driver/mysql memanggil
// ini secara rutin, sedangkan channel SSH mentah tidak selalu mendukungnya).
func (e *Executor) DialTunnel(ctx context.Context, serverID, network, addr string) (net.Conn, error) {
	conn, err := e.pool.Acquire(ctx, serverID, SlotShared)
	if err != nil {
		return nil, err
	}
	defer e.pool.Release(serverID, SlotShared, conn)
	c, err := conn.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return &nopDeadlineConn{Conn: c}, nil
}

// nopDeadlineConn membuat Set(Read/Write)Deadline jadi no-op (tidak pernah
// gagal) untuk net.Conn yang tidak benar-benar mendukung deadline.
type nopDeadlineConn struct {
	net.Conn
}

func (c *nopDeadlineConn) SetDeadline(t time.Time) error      { return nil }
func (c *nopDeadlineConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *nopDeadlineConn) SetWriteDeadline(t time.Time) error { return nil }
