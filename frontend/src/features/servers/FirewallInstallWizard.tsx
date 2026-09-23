import { useEffect, useRef, useState } from 'react';
import { ShieldPlus, TriangleAlert, Info } from 'lucide-react';
import { GetFirewallInstallInfo, StreamFirewallInstall, StopDockerStream } from '../../../wailsjs/go/main/App';
import { firewall } from '../../../wailsjs/go/models';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import { useT } from '../../i18n';

interface StreamLineEvent {
  type: 'line' | 'end' | 'error';
  line?: string;
  message?: string;
}

// Batas baris log yang disimpan — output apt/dnf bisa ribuan baris.
const MAX_LINES = 2000;

// Ditampilkan menggantikan panel Firewall kalau server belum punya firewall
// sama sekali. Backend yang ditawarkan disesuaikan dengan distro (lihat
// internal/modules/firewall/install.go): ufw untuk keluarga Debian/Ubuntu,
// Arch, Alpine; firewalld untuk keluarga Red Hat & SUSE.
//
// Instalasi TIDAK menyalakan firewall. Sesudahnya panel Firewall biasa
// muncul dalam keadaan mati, dan tombol Aktifkan di sana yang menyalakannya
// (selalu mengizinkan port SSH lebih dulu).
export function FirewallInstallWizard({ serverId, onInstalled }: { serverId: string; onInstalled: () => void }) {
  const t = useT();
  const [info, setInfo] = useState<firewall.InstallInfo | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [backend, setBackend] = useState('');
  const [lines, setLines] = useState<string[]>([]);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const streamIdRef = useRef<string | null>(null);
  const unsubRef = useRef<(() => void) | null>(null);
  const logRef = useRef<HTMLPreElement>(null);

  useEffect(() => {
    let stale = false;
    setInfo(null);
    setLoadError(null);
    GetFirewallInstallInfo(serverId)
      .then((i) => {
        if (stale) return;
        setInfo(i);
        setBackend(i.recommended || i.options[0] || '');
      })
      .catch((e) => {
        if (!stale) setLoadError(String(e));
      });
    return () => {
      stale = true;
    };
  }, [serverId]);

  // Unmount (tab ditutup / pindah server): hentikan stream DAN lepas
  // listener-nya — kalau tidak, listener tetap hidup dan memanggil setState
  // pada komponen yang sudah tidak ada.
  useEffect(() => {
    return () => {
      unsubRef.current?.();
      if (streamIdRef.current) void StopDockerStream(streamIdRef.current);
    };
  }, []);

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight });
  }, [lines]);

  function finish() {
    unsubRef.current?.();
    unsubRef.current = null;
    streamIdRef.current = null;
    setRunning(false);
  }

  async function handleInstall() {
    setLines([]);
    setError(null);
    setRunning(true);
    try {
      const streamId = await StreamFirewallInstall(serverId, backend);
      streamIdRef.current = streamId;
      unsubRef.current = EventsOn(`firewall:install:${streamId}`, (evt: StreamLineEvent) => {
        if (evt.type === 'line' && evt.line !== undefined) {
          setLines((l) => {
            const next = [...l, evt.line as string];
            return next.length > MAX_LINES ? next.slice(-MAX_LINES) : next;
          });
        } else if (evt.type === 'error') {
          setError(evt.message ?? t('fw.install.failed'));
          finish();
        } else if (evt.type === 'end') {
          finish();
          onInstalled();
        }
      });
    } catch (e) {
      setError(String(e));
      finish();
    }
  }

  if (loadError) {
    return (
      <div className="docker-engine">
        <p className="overview__error">{loadError}</p>
      </div>
    );
  }
  if (!info) return <p className="workspace__placeholder">{t('common.loading')}</p>;

  return (
    <div className="docker-engine fw-install">
      <p>
        <TriangleAlert size={13} /> {t('fw.noFirewall')}
      </p>
      <p className="fw-install__distro">
        {t('fw.install.distro', { distro: info.distroName, pm: info.packageManager || '—' })}
      </p>

      {info.blocker && <p className="overview__error">{info.blocker}</p>}
      {!info.blocker && info.options.length === 0 && (
        <p className="overview__error">{t('fw.install.unsupported')}</p>
      )}

      {info.canInstall && (
        <>
          <div className="fw-install__options">
            {info.options.map((o) => (
              <label key={o} className="fw-install__option">
                <input
                  type="radio"
                  name={`fw-backend-${serverId}`}
                  value={o}
                  checked={backend === o}
                  disabled={running}
                  onChange={() => setBackend(o)}
                />
                <span className="fw-install__name">{o}</span>
                {o === info.recommended && <span className="fw-install__badge">{t('fw.install.recommended')}</span>}
                <span className="fw-install__desc">{t(o === 'ufw' ? 'fw.install.ufwDesc' : 'fw.install.firewalldDesc')}</span>
              </label>
            ))}
          </div>
          <p className="fw-install__note">
            <Info size={13} /> {t('fw.install.safety', { port: String(info.sshPort) })}
          </p>
          {info.hasDocker && (
            <p className="fw-install__note">
              <Info size={13} /> {t(backend === 'ufw' ? 'fw.install.dockerUfw' : 'fw.install.dockerFirewalld')}
            </p>
          )}
          <button className="btn btn--primary" disabled={running || !backend} onClick={() => void handleInstall()}>
            <ShieldPlus size={13} /> {running ? t('fw.install.installing') : t('fw.install.button', { backend })}
          </button>
        </>
      )}

      {error && <p className="overview__error">{error}</p>}
      {lines.length > 0 && (
        <pre className="docker-engine__log" ref={logRef}>
          {lines.join('\n')}
        </pre>
      )}
    </div>
  );
}
