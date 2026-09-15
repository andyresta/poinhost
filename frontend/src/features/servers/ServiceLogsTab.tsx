import { useEffect, useMemo, useRef, useState } from 'react';
import { RotateCw, Play, Square, TriangleAlert } from 'lucide-react';
import { ReadSystemJournal, StreamSystemJournal, StopDockerStream } from '../../../wailsjs/go/main/App';
import { services } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { useT } from '../../i18n';

interface JournalStreamEvent {
  type: 'entry' | 'end' | 'error';
  entry?: services.JournalEntry;
  message?: string;
}

// Nilai filter SENGAJA dibatasi ke daftar tetap yang sama persis dengan
// whitelist di backend (journal.go). Tidak ada input teks bebas yang masuk ke
// perintah journalctl.
const SINCE_OPTIONS = ['15m', '1h', '6h', '24h', '7d', 'boot'] as const;
const PRIORITY_OPTIONS = ['', 'err', 'warning', 'info', 'debug'] as const;
const LINE_OPTIONS = [200, 500, 1000, 2000] as const;

// Level syslog: 0 emerg … 7 debug. Dikelompokkan jadi tiga warna saja —
// lebih dari itu justru sulit dibaca sekilas.
function priorityClass(priority: number) {
  if (priority <= 3) return 'svc-journal__line--err';
  if (priority === 4) return 'svc-journal__line--warn';
  if (priority >= 7) return 'svc-journal__line--debug';
  return '';
}

function shortTime(iso: string) {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleTimeString(undefined, { hour12: false });
}

// Tab Logs di modul Services — pembaca journald.
//
// Alurnya sama dengan log domain/container: ambil snapshot dulu lewat
// ReadSystemJournal, lalu (opsional) sambung ke `journalctl -f` untuk baris
// baru. Follow dibuat OPT-IN, tidak otomatis, karena membuka koneksi SSH
// khusus yang menganggur selama tab ini terbuka.
export function ServiceLogsTab({
  serverId,
  units,
  initialUnit,
}: {
  serverId: string;
  units: string[];
  initialUnit?: string;
}) {
  const t = useT();
  const [unit, setUnit] = useState(initialUnit ?? '');
  const [priority, setPriority] = useState<string>('');
  const [since, setSince] = useState<string>('1h');
  const [lines, setLines] = useState<number>(500);
  const [query, setQuery] = useState('');

  const [entries, setEntries] = useState<services.JournalEntry[]>([]);
  const [journalAvailable, setJournalAvailable] = useState(true);
  const [truncated, setTruncated] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [following, setFollowing] = useState(false);

  const streamIdRef = useRef<string | null>(null);
  // Penjaga generasi. Mengubah filter memicu load() baru sementara yang lama
  // masih jalan, dan permintaan TANPA filter jauh lebih lambat daripada yang
  // terfilter — tanpa penjaga ini hasil lama bisa mendarat belakangan dan
  // menimpa hasil filter yang baru, sehingga layar menampilkan data yang
  // tidak sesuai dengan pilihan filter yang terlihat.
  const loadIdRef = useRef(0);
  const logRef = useRef<HTMLDivElement>(null);
  // Menempel di dasar hanya kalau user MEMANG sedang di dasar — kalau dia
  // sedang menggulir ke atas membaca baris lama, baris baru tidak boleh
  // menyeretnya kembali ke bawah.
  const stickToBottomRef = useRef(true);

  // Pindah server / ganti unit lewat tombol "Logs" di daftar service.
  useEffect(() => {
    if (initialUnit !== undefined) setUnit(initialUnit);
  }, [initialUnit]);

  async function load() {
    const gen = ++loadIdRef.current;
    setLoading(true);
    setError(null);
    try {
      const res = await ReadSystemJournal(
        new services.JournalRequest({ serverId, unit, priority, since, lines }),
      );
      if (gen !== loadIdRef.current) return; // sudah ada permintaan yang lebih baru
      setEntries(res.entries ?? []);
      setJournalAvailable(res.systemdAvailable);
      setTruncated(res.truncated);
      stickToBottomRef.current = true;
    } catch (e) {
      if (gen !== loadIdRef.current) return;
      setError(String(e));
    } finally {
      if (gen === loadIdRef.current) setLoading(false);
    }
  }

  // Setiap perubahan filter mengambil ulang snapshot DAN memutus follow yang
  // sedang berjalan — stream lama memakai filter lama, jadi membiarkannya
  // hidup akan mencampur dua hasil filter berbeda di satu layar.
  useEffect(() => {
    setFollowing(false);
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, unit, priority, since, lines]);

  useEffect(() => {
    if (!following) {
      if (streamIdRef.current) {
        void StopDockerStream(streamIdRef.current);
        streamIdRef.current = null;
      }
      return;
    }

    let cancelled = false;
    let unsub: (() => void) | null = null;

    void (async () => {
      try {
        const streamId = await StreamSystemJournal(
          new services.JournalRequest({ serverId, unit, priority, since, lines: 0 }),
        );
        if (cancelled) {
          void StopDockerStream(streamId);
          return;
        }
        streamIdRef.current = streamId;
        unsub = EventsOn(`services:journal:${streamId}`, (evt: JournalStreamEvent) => {
          if (evt.type === 'entry' && evt.entry) {
            const entry = evt.entry;
            // Dibatasi supaya sesi follow yang panjang tidak menumpuk
            // ribuan node DOM sampai panelnya berat.
            setEntries((prev) => [...prev, entry].slice(-5000));
          } else if (evt.type === 'error') {
            setError(evt.message ?? 'stream log terputus');
            setFollowing(false);
          } else if (evt.type === 'end') {
            setFollowing(false);
          }
        });
      } catch (e) {
        if (!cancelled) {
          setError(String(e));
          setFollowing(false);
        }
      }
    })();

    return () => {
      cancelled = true;
      unsub?.();
      if (streamIdRef.current) {
        void StopDockerStream(streamIdRef.current);
        streamIdRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [following]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return entries;
    return entries.filter(
      (e) => e.message.toLowerCase().includes(q) || (e.unit ?? '').toLowerCase().includes(q),
    );
  }, [entries, query]);

  useEffect(() => {
    if (stickToBottomRef.current && logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [filtered]);

  function onScroll() {
    const el = logRef.current;
    if (!el) return;
    stickToBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
  }

  if (!journalAvailable) {
    return (
      <div className="docker-engine">
        <p>
          <TriangleAlert size={13} /> {t('journal.unavailable')}
        </p>
      </div>
    );
  }

  return (
    <div className="files-panel">
      <div className="files-panel__toolbar">
        <select value={unit} onChange={(e) => setUnit(e.target.value)} style={{ maxWidth: 220 }}>
          <option value="">{t('journal.allUnits')}</option>
          {units.map((u) => (
            <option key={u} value={u}>
              {u}
            </option>
          ))}
        </select>

        <select value={priority} onChange={(e) => setPriority(e.target.value)}>
          {PRIORITY_OPTIONS.map((p) => (
            <option key={p || 'all'} value={p}>
              {t(`journal.priority.${p || 'all'}` as 'journal.priority.all')}
            </option>
          ))}
        </select>

        <select value={since} onChange={(e) => setSince(e.target.value)}>
          {SINCE_OPTIONS.map((sv) => (
            <option key={sv} value={sv}>
              {t(`journal.since.${sv}` as 'journal.since.1h')}
            </option>
          ))}
        </select>

        <select value={lines} onChange={(e) => setLines(Number(e.target.value))}>
          {LINE_OPTIONS.map((n) => (
            <option key={n} value={n}>
              {t('journal.lines', { n: String(n) })}
            </option>
          ))}
        </select>

        <button className="btn btn--sm" disabled={loading} onClick={() => void load()}>
          {loading ? <span className="spinner" /> : <RotateCw size={13} />} {t('common.refresh')}
        </button>

        <button
          className={`btn btn--sm${following ? ' btn--active' : ''}`}
          onClick={() => setFollowing((f) => !f)}
        >
          {following ? <Square size={13} /> : <Play size={13} />}{' '}
          {following ? t('journal.stopFollow') : t('journal.follow')}
        </button>
        {following && <span className="docker-live-dot" title={t('journal.live')} />}

        <input
          placeholder={t('journal.search')}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          style={{ marginLeft: 'auto', maxWidth: 200 }}
        />
      </div>

      {error && <p className="overview__error">{error}</p>}
      {truncated && !following && <p className="svc-journal__note">{t('journal.truncated')}</p>}

      <div className="svc-journal" ref={logRef} onScroll={onScroll}>
        {filtered.map((e, i) => (
          <div key={`${e.timestamp}-${i}`} className={`svc-journal__line ${priorityClass(e.priority)}`}>
            <span className="svc-journal__time">{shortTime(e.timestamp)}</span>
            <span className="svc-journal__unit">{e.unit || e.identifier || '—'}</span>
            <span className="svc-journal__msg">{e.message}</span>
          </div>
        ))}
        {!loading && filtered.length === 0 && (
          <div className="svc-journal__empty">{t('journal.noEntries')}</div>
        )}
      </div>
    </div>
  );
}
