import { useEffect, useState } from 'react';
import { X } from 'lucide-react';
import { InspectDockerContainer, RecreateDockerContainer } from '../../../wailsjs/go/main/App';
import { docker } from '../../../wailsjs/go/models';
import { useConfirm } from '../../components/ConfirmDialog';
import { useT } from '../../i18n';

// Docker tidak punya "edit container in place" — satu-satunya cara mengubah
// env/port/volume/memory container yang sudah ada adalah stop -> rm ->
// create (dengan konfigurasi baru) -> start (lihat
// internal/modules/docker/service_config.go RecreateContainer). Modal ini
// memuat konfigurasi SEKARANG lewat InspectDockerContainer sebagai starting
// point, biar user cuma perlu mengubah bagian yang mau diganti.
export function RecreateContainerModal({
  serverId,
  containerId,
  name,
  onClose,
  onDone,
}: {
  serverId: string;
  containerId: string;
  name: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const t = useT();
  const confirm = useConfirm();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [env, setEnv] = useState<docker.EnvVar[]>([]);
  const [ports, setPorts] = useState<docker.PortMapping[]>([]);
  const [volumes, setVolumes] = useState<docker.VolumeMount[]>([]);
  const [memoryMB, setMemoryMB] = useState(0);

  useEffect(() => {
    let cancelled = false;
    InspectDockerContainer(serverId, containerId)
      .then((detail) => {
        if (cancelled) return;
        setEnv(detail.env);
        setPorts(detail.ports);
        setVolumes(detail.volumes);
        setMemoryMB(detail.memoryBytes > 0 ? Math.round(detail.memoryBytes / (1024 * 1024)) : 0);
      })
      .catch((e) => !cancelled && setError(String(e)))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [serverId, containerId]);

  async function handleSave() {
    const ok = await confirm({
      title: t('rc.title'),
      message: t('rc.confirm', { name }),
      detail: t('rc.detail'),
      confirmLabel: t('rc.apply'),
    });
    if (!ok) return;
    setSaving(true);
    setError(null);
    try {
      const res = await RecreateDockerContainer(
        new docker.RecreateContainerRequest({
          serverId,
          containerId,
          env,
          ports,
          volumes,
          memoryBytes: memoryMB > 0 ? memoryMB * 1024 * 1024 : 0,
        }),
      );
      // Peringatan firewall sudah tidak ada lagi: scope ditegakkan lewat
      // alamat bind, bukan aturan firewall yang bisa saja tidak berlaku.
      onDone();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  }

  // Dua pilihan saja. "Private" ditegakkan lewat alamat bind (127.0.0.1),
  // bukan aturan firewall — jadi berlaku apa pun kondisi firewall server.
  function scopeLabel(scope: string) {
    if (scope === 'private') return t('rc.scopePrivate');
    return t('rc.scopePublic');
  }

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--editor">
        <div className="modal-card__header">
          <h2>Konfigurasi — {name}</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body modal-card__body--editor docker-recreate">
          {loading ? (
            <p className="workspace__placeholder">{t('rc.loadingConfig')}</p>
          ) : (
            <>
              {error && <p className="overview__error">{error}</p>}

              <section>
                <div className="docker-recreate__section-head">
                  <h3>{t('rc.environment')}</h3>
                  <button className="btn btn--sm" onClick={() => setEnv([...env, new docker.EnvVar({ key: '', value: '' })])}>
                    + Tambah
                  </button>
                </div>
                {env.map((e, i) => (
                  <div className="docker-recreate__row" key={i}>
                    <input
                      placeholder="KEY"
                      value={e.key}
                      onChange={(ev) => setEnv(env.map((x, j) => (j === i ? new docker.EnvVar({ ...x, key: ev.target.value }) : x)))}
                    />
                    <input
                      placeholder="value"
                      value={e.value}
                      onChange={(ev) => setEnv(env.map((x, j) => (j === i ? new docker.EnvVar({ ...x, value: ev.target.value }) : x)))}
                    />
                    <button className="btn btn--sm btn--danger" onClick={() => setEnv(env.filter((_, j) => j !== i))}>
                      <X size={14} />
                    </button>
                  </div>
                ))}
              </section>

              <section>
                <div className="docker-recreate__section-head">
                  <h3>{t('rc.ports')}</h3>
                  <button
                    className="btn btn--sm"
                    onClick={() => setPorts([...ports, new docker.PortMapping({ hostPort: 0, containerPort: 0, protocol: 'tcp', scope: 'public' })])}
                  >
                    + Tambah
                  </button>
                </div>
                {ports.map((p, i) => (
                  <div className="docker-recreate__row" key={i}>
                    <input
                      type="number"
                      placeholder={t('rc.phHost')}
                      value={p.hostPort || ''}
                      onChange={(ev) =>
                        setPorts(ports.map((x, j) => (j === i ? new docker.PortMapping({ ...x, hostPort: Number(ev.target.value) }) : x)))
                      }
                    />
                    <span>:</span>
                    <input
                      type="number"
                      placeholder={t('rc.phContainer')}
                      value={p.containerPort || ''}
                      onChange={(ev) =>
                        setPorts(
                          ports.map((x, j) => (j === i ? new docker.PortMapping({ ...x, containerPort: Number(ev.target.value) }) : x)),
                        )
                      }
                    />
                    <select
                      value={p.protocol}
                      onChange={(ev) => setPorts(ports.map((x, j) => (j === i ? new docker.PortMapping({ ...x, protocol: ev.target.value }) : x)))}
                    >
                      <option value="tcp">tcp</option>
                      <option value="udp">udp</option>
                    </select>
                    <select
                      title={t('rc.scopeHint')}
                      value={p.scope || 'public'}
                      onChange={(ev) => setPorts(ports.map((x, j) => (j === i ? new docker.PortMapping({ ...x, scope: ev.target.value }) : x)))}
                    >
                      <option value="public">{scopeLabel('public')}</option>
                      <option value="private">{scopeLabel('private')}</option>
                    </select>
                    <button className="btn btn--sm btn--danger" onClick={() => setPorts(ports.filter((_, j) => j !== i))}>
                      <X size={14} />
                    </button>
                  </div>
                ))}
                {ports.length > 0 && (
                  <p className="chmod-path">
                    Publik = bisa diakses dari internet. Intranet = hanya dari jaringan lokal (dibatasi lewat firewall server, kalau aktif).
                    Localhost = hanya dari server itu sendiri.
                  </p>
                )}
              </section>

              <section>
                <div className="docker-recreate__section-head">
                  <h3>{t('rc.volumes')}</h3>
                  <button
                    className="btn btn--sm"
                    onClick={() => setVolumes([...volumes, new docker.VolumeMount({ hostPath: '', containerPath: '', readOnly: false })])}
                  >
                    + Tambah
                  </button>
                </div>
                {volumes.map((v, i) => (
                  <div className="docker-recreate__row" key={i}>
                    <input
                      placeholder="/host/path"
                      value={v.hostPath}
                      onChange={(ev) =>
                        setVolumes(volumes.map((x, j) => (j === i ? new docker.VolumeMount({ ...x, hostPath: ev.target.value }) : x)))
                      }
                    />
                    <span>:</span>
                    <input
                      placeholder="/container/path"
                      value={v.containerPath}
                      onChange={(ev) =>
                        setVolumes(volumes.map((x, j) => (j === i ? new docker.VolumeMount({ ...x, containerPath: ev.target.value }) : x)))
                      }
                    />
                    <label className="docker-recreate__ro">
                      <input
                        type="checkbox"
                        checked={v.readOnly}
                        onChange={(ev) =>
                          setVolumes(volumes.map((x, j) => (j === i ? new docker.VolumeMount({ ...x, readOnly: ev.target.checked }) : x)))
                        }
                      />
                      ro
                    </label>
                    <button className="btn btn--sm btn--danger" onClick={() => setVolumes(volumes.filter((_, j) => j !== i))}>
                      <X size={14} />
                    </button>
                  </div>
                ))}
              </section>

              <section>
                <h3>{t('rc.memoryLimit')}</h3>
                <input
                  type="number"
                  min={0}
                  value={memoryMB}
                  onChange={(ev) => setMemoryMB(Number(ev.target.value))}
                  className="docker-recreate__memory"
                />
              </section>
            </>
          )}
        </div>
        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={onClose}>
            Batal
          </button>
          <button className="btn btn--primary" disabled={loading || saving} onClick={() => void handleSave()}>
            {saving && <span className="spinner" />} {saving ? t('rc.applying') : t('rc.applyRecreate')}
          </button>
        </div>
      </div>
    </div>
  );
}
