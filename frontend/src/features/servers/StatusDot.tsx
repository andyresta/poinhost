import { useT } from '../../i18n';

// Key i18n, bukan teks jadi — supaya label ikut berubah saat bahasa diganti
// tanpa perlu reload.
const LABEL_KEYS: Record<string, 'status.online' | 'status.offline' | 'status.reconnecting'> = {
  online: 'status.online',
  offline: 'status.offline',
  reconnecting: 'status.reconnecting',
};

// Titik status koneksi — bersumber dari sshpool.Pool via Collector (lihat
// ARCHITECTURE.md §8), bukan hasil polling frontend. `connection` kosong
// berarti belum pernah dicek sama sekali (server baru ditambahkan, belum
// sempat di-poll connectionLoop pertama kali).
export function StatusDot({ connection }: { connection?: string }) {
  const t = useT();
  const state = connection || 'unknown';
  const key = LABEL_KEYS[state];
  return <span className={`status-dot status-dot--${state}`} title={key ? t(key) : t('status.unknown')} />;
}
