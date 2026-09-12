const LABELS: Record<string, string> = {
  online: 'Online',
  offline: 'Offline',
  reconnecting: 'Menyambung ulang…',
};

// Titik status koneksi — bersumber dari sshpool.Pool via Collector (lihat
// ARCHITECTURE.md §8), bukan hasil polling frontend. `connection` kosong
// berarti belum pernah dicek sama sekali (server baru ditambahkan, belum
// sempat di-poll connectionLoop pertama kali).
export function StatusDot({ connection }: { connection?: string }) {
  const state = connection || 'unknown';
  return (
    <span
      className={`status-dot status-dot--${state}`}
      title={LABELS[state] ?? 'Status belum diketahui'}
    />
  );
}
