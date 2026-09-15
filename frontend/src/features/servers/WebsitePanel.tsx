import { Fragment, useEffect, useState } from 'react';
import { RotateCw, CornerDownRight, Lock, Settings, Plus, Pause, Play, Trash2, X } from 'lucide-react';
import { ListWebsites, CreateWebsiteSubdomain, DeleteWebsite, SetWebsiteEnabled } from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';
import { WebsiteEngineWizard } from './WebsiteEngineWizard';
import { CreateWebsiteModal } from './CreateWebsiteModal';
import { SubdomainModal } from './SubdomainModal';
import { DomainDetailPanel } from './DomainDetailPanel';
import { DomainFeatureGrid, type FeatureKey } from './DomainFeatureGrid';
import { useT } from '../../i18n';

// Menu Website untuk SATU tab — mengelola domain/vhost Nginx, PHP-FPM, dan
// SSL Let's Encrypt di server. Mekanismenya diporting dari homepoin (sudah
// production-proven di sana), tapi disusun ulang agar tidak pernah
// menjalankan lebih dari satu round-trip SSH per tampilan (lihat
// ARCHITECTURE.md §Website untuk diagnosis kenapa homepoin terasa lambat
// pindah-pindah menu di sini — bukan soal SSH re-handshake, yang di
// poinhost memang tidak pernah terjadi lagi sejak awal).
export function WebsitePanel({ serverId }: { serverId: string }) {
  const t = useT();
  const [list, setList] = useState<website.ListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  // refreshing dipisah dari loading: loading menggantikan SELURUH panel
  // dengan layar "Memuat…" (hanya pantas untuk load pertama), sedangkan
  // refresh manual harus tetap menampilkan daftar lama + spinner di
  // tombolnya. Sebelumnya load() tidak pernah menyalakan flag apa pun di
  // AWAL, jadi klik Refresh terasa tidak melakukan apa-apa.
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busyDomain, setBusyDomain] = useState<string | null>(null);
  const [query, setQuery] = useState('');

  const [showCreate, setShowCreate] = useState(false);
  const [createdNotice, setCreatedNotice] = useState<website.CreateWebsiteResult | null>(null);
  const [subdomainParent, setSubdomainParent] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<website.DomainInfo | null>(null);
  const [removeRoot, setRemoveRoot] = useState(false);
  const [detailDomain, setDetailDomain] = useState<string | null>(null);
  // Alur ala Plesk: tombol gear TIDAK langsung pindah halaman — ia membuka
  // grid ikon fitur INLINE di bawah baris domainnya (expandedDomain), baru
  // setelah satu ikon dipilih berpindah ke halaman fitur itu
  // (detailDomain + detailFeature).
  const [expandedDomain, setExpandedDomain] = useState<string | null>(null);
  const [detailFeature, setDetailFeature] = useState<FeatureKey | null>(null);

  async function load() {
    setError(null);
    setRefreshing(true);
    try {
      setList(await ListWebsites(serverId));
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    setLoading(true);
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId]);

  const domains = list?.domains ?? [];
  const filtered = query.trim()
    ? domains.filter((d) => d.domain.toLowerCase().includes(query.trim().toLowerCase()))
    : domains;

  // Susun hierarkis: parent dulu, subdomain-nya langsung di bawah (indented).
  const parents = filtered.filter((d) => !d.isSubdomain);
  const orphanSubs = filtered.filter((d) => d.isSubdomain && !parents.some((p) => p.domain === d.parent));
  const rows: { domain: website.DomainInfo; indent: boolean }[] = [];
  for (const p of parents) {
    rows.push({ domain: p, indent: false });
    for (const child of filtered.filter((d) => d.isSubdomain && d.parent === p.domain)) {
      rows.push({ domain: child, indent: true });
    }
  }
  for (const orphan of orphanSubs) rows.push({ domain: orphan, indent: false });

  const detailInfo = detailDomain ? domains.find((d) => d.domain === detailDomain) ?? null : null;

  async function toggleEnabled(d: website.DomainInfo) {
    setBusyDomain(d.domain);
    setError(null);
    try {
      await SetWebsiteEnabled(new website.SetEnabledRequest({ serverId, domain: d.domain, enabled: !d.enabled }));
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusyDomain(null);
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    setBusyDomain(deleteTarget.domain);
    setError(null);
    try {
      await DeleteWebsite(new website.DeleteDomainRequest({ serverId, domain: deleteTarget.domain, removeRoot }));
      if (detailDomain === deleteTarget.domain) setDetailDomain(null);
      setDeleteTarget(null);
      setRemoveRoot(false);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusyDomain(null);
    }
  }

  async function handleCreateSubdomain(label: string) {
    if (!subdomainParent) return;
    await CreateWebsiteSubdomain(new website.CreateSubdomainRequest({ serverId, parent: subdomainParent, subdomain: label }));
    await load();
  }

  if (loading) return <p className="workspace__placeholder">{t('common.loading')}</p>;

  const nginx = list?.nginx;

  return (
    <div className="files-panel">
      {error && <p className="overview__error">{error}</p>}

      {nginx && (!nginx.installed || !nginx.active) && (
        <WebsiteEngineWizard serverId={serverId} status={nginx} onChange={() => void load()} />
      )}

      {nginx?.installed && nginx.active && detailInfo && (
        <DomainDetailPanel
          serverId={serverId}
          domain={detailInfo}
          initialFeature={detailFeature}
          onBack={() => {
            setDetailDomain(null);
            setDetailFeature(null);
          }}
          onChanged={() => void load()}
          onToggleEnabled={() => void toggleEnabled(detailInfo)}
          onAddSubdomain={() => setSubdomainParent(detailInfo.domain)}
          onDelete={() => setDeleteTarget(detailInfo)}
          busy={busyDomain === detailInfo.domain}
        />
      )}

      {nginx?.installed && nginx.active && !detailInfo && (
        <>
          <div className="files-panel__toolbar">
            <button className="btn btn--sm" disabled={refreshing} onClick={() => void load()}>
              {refreshing ? <span className="spinner" /> : <RotateCw size={13} />} {t('common.refresh')}
            </button>
            <button className="btn btn--sm btn--primary" onClick={() => setShowCreate(true)}>
              {t('web.createWebsite')}
            </button>
            <input
              placeholder={t('web.searchDomain')}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              style={{ marginLeft: 'auto', maxWidth: 220 }}
            />
            {nginx.version && <span className="docker-panel__version">Nginx {nginx.version}</span>}
          </div>

          {createdNotice && (
            <div className="answer-like-notice">
              <p>
                Website <strong>{createdNotice.domain.domain}</strong> berhasil dibuat.
              </p>
              {createdNotice.warnings && createdNotice.warnings.length > 0 && (
                <ul>
                  {createdNotice.warnings.map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                </ul>
              )}
              <button className="btn btn--ghost btn--sm" onClick={() => setCreatedNotice(null)}>
                Tutup
              </button>
            </div>
          )}

          <div className="files-panel__table-wrap">
            <table className="files-panel__table">
              <thead>
                <tr>
                  <th>{t('web.domain')}</th>
                  <th>PHP</th>
                  <th>SSL</th>
                  <th>{t('common.status')}</th>
                  <th className="files-panel__col-actions" />
                </tr>
              </thead>
              <tbody>
                {rows.map(({ domain: d, indent }) => (
                  <Fragment key={d.domain}>
                    <tr>
                      <td className="files-panel__name" style={indent ? { paddingLeft: 28 } : undefined}>
                        {indent && <CornerDownRight size={12} />}
                        {d.domain}
                      </td>
                      <td>{d.phpEnabled ? `PHP ${d.phpVersion}` : '—'}</td>
                      <td>{d.sslEnabled ? (<><Lock size={12} /> Aktif</>) : '—'}</td>
                      <td>
                        <span className={`docker-badge ${d.enabled ? 'docker-badge--running' : 'docker-badge--stopped'}`}>
                          {d.enabled ? t('common.active') : t('common.inactive')}
                        </span>
                      </td>
                      <td className="files-panel__row-actions">
                        <button
                          title={expandedDomain === d.domain ? t('web.closeMenu') : t('web.manage')}
                          onClick={() => setExpandedDomain(expandedDomain === d.domain ? null : d.domain)}
                        >
                          <Settings size={14} />
                        </button>
                        {!d.isSubdomain && (
                          <button title={t('web.addSubdomain')} onClick={() => setSubdomainParent(d.domain)}>
                            <Plus size={14} />
                          </button>
                        )}
                        <button
                          title={d.enabled ? t('web.disable') : t('web.enable')}
                          disabled={busyDomain === d.domain}
                          onClick={() => void toggleEnabled(d)}
                        >
                          {busyDomain === d.domain ? <span className="spinner" /> : d.enabled ? <Pause size={14} /> : <Play size={14} />}
                        </button>
                        <button title={t('common.delete')} disabled={busyDomain === d.domain} onClick={() => setDeleteTarget(d)}>
                          <Trash2 size={14} />
                        </button>
                      </td>
                    </tr>
                    {expandedDomain === d.domain && (
                      <tr className="domain-row-expansion">
                        <td colSpan={5}>
                          <DomainFeatureGrid
                            isSubdomain={d.isSubdomain}
                            onSelect={(key) => {
                              setDetailFeature(key);
                              setDetailDomain(d.domain);
                              setExpandedDomain(null);
                            }}
                          />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                ))}
                {rows.length === 0 && (
                  <tr>
                    <td colSpan={5} className="files-panel__empty">
                      Belum ada website. Klik <strong>+ Buat Website</strong> untuk mulai.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </>
      )}

      {showCreate && (
        <CreateWebsiteModal
          serverId={serverId}
          onClose={() => setShowCreate(false)}
          onDone={(result) => {
            setShowCreate(false);
            setCreatedNotice(result);
            void load();
          }}
        />
      )}

      {subdomainParent && (
        <SubdomainModal parent={subdomainParent} onClose={() => setSubdomainParent(null)} onConfirm={handleCreateSubdomain} />
      )}

      {deleteTarget && (
        <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && setDeleteTarget(null)}>
          <div className="modal-card modal-card--small">
            <div className="modal-card__header">
              <h2>Hapus Website</h2>
              <button className="modal-card__close" onClick={() => setDeleteTarget(null)}>
                <X size={18} />
              </button>
            </div>
            <div className="modal-card__body">
              <p>
                Hapus <strong>{deleteTarget.domain}</strong>? Konfigurasi vhost-nya akan dihapus dan Nginx di-reload.
              </p>
              <label className="form-check">
                <input type="checkbox" checked={removeRoot} onChange={(e) => setRemoveRoot(e.target.checked)} />
                <span>Hapus juga document root ({deleteTarget.root}) beserta seluruh isinya</span>
              </label>
            </div>
            <div className="modal-card__footer">
              <button className="btn btn--ghost" onClick={() => setDeleteTarget(null)}>
                Batal
              </button>
              <button className="btn btn--danger" disabled={busyDomain === deleteTarget.domain} onClick={() => void handleDelete()}>
                {busyDomain === deleteTarget.domain && <span className="spinner" />} Hapus
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
