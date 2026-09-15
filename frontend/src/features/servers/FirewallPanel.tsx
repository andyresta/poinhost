import { useEffect, useState } from 'react';
import { RotateCw, Plus, Trash2, ShieldCheck, ShieldOff, TriangleAlert, Lock } from 'lucide-react';
import {
  ListFirewallRules,
  AddFirewallRule,
  DeleteFirewallRule,
  SetFirewallEnabled,
} from '../../../wailsjs/go/main/App';
import { firewall } from '../../../wailsjs/go/models';
import { useConfirm } from '../../components/ConfirmDialog';
import { useT } from '../../i18n';
import { FirewallRuleModal } from './FirewallRuleModal';

// Modul Firewall.
//
// Backend ditentukan dari yang SEDANG AKTIF di server (ufw / firewalld /
// nftables), bukan ditebak dari distro — satu server bisa punya ketiganya
// terpasang sekaligus. ufw dan firewalld bisa diubah dari sini; nftables
// mentah hanya dilaporkan, tidak diubah.
export function FirewallPanel({ serverId }: { serverId: string }) {
  const t = useT();
  const confirm = useConfirm();
  // State diurai, bukan menyimpan objek ListResponse utuh: kelas hasil
  // generate Wails membawa method convertValues yang akan hilang kalau
  // objeknya di-spread untuk update sebagian.
  const [status, setStatus] = useState<firewall.Status | null>(null);
  const [rules, setRules] = useState<firewall.Rule[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  // Zone hanya berlaku di firewalld. Kosong = zone default server; nilainya
  // baru terisi kalau user memilih sendiri, supaya kita tidak memaksa
  // --zone= dengan tebakan.
  const [zone, setZone] = useState('');

  function apply(res: firewall.ListResponse) {
    setStatus(res.status);
    setRules(res.rules ?? []);
  }

  async function load() {
    setError(null);
    setRefreshing(true);
    try {
      apply(await ListFirewallRules(serverId, zone));
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
  }, [serverId, zone]);

  async function handleDelete(rule: firewall.Rule) {
    const ok = await confirm({
      title: t('fw.deleteRule'),
      message: t('fw.deleteConfirm', { rule: ruleLabel(rule) }),
      // Aturan buatan fitur lain diberi peringatan terpisah: menghapusnya di
      // sini akan dipasang ulang saat fitur asalnya dijalankan lagi.
      detail: rule.owner ? t('fw.deleteOwnedDetail', { owner: rule.owner }) : t('fw.deleteDetail'),
      confirmLabel: t('common.delete'),
      danger: true,
    });
    if (!ok) return;
    setBusy(true);
    setError(null);
    try {
      apply(await DeleteFirewallRule(serverId, rule.id, zone));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleToggleEnabled() {
    if (!status) return;
    const turningOn = !status.active;

    // Pengaman anti-terkunci. Backend tetap memasang aturan SSH lebih dulu,
    // tapi konfirmasinya menyebut port yang dipakai secara eksplisit supaya
    // user tahu persis apa yang akan terjadi pada sesinya sendiri.
    const ok = await confirm({
      title: turningOn ? t('fw.enable') : t('fw.disable'),
      message: turningOn
        ? t('fw.enableConfirm', { backend: status.backend })
        : t('fw.disableConfirm', { backend: status.backend }),
      detail: turningOn
        ? t('fw.enableDetail', { port: String(status.sshPort) })
        : t('fw.disableDetail'),
      confirmLabel: turningOn ? t('fw.enable') : t('fw.disable'),
      danger: true,
    });
    if (!ok) return;

    setBusy(true);
    setError(null);
    try {
      apply(await SetFirewallEnabled(serverId, turningOn));
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleAdd(rule: {
    port: string;
    protocol: string;
    source: string;
    action: string;
    comment: string;
  }) {
    apply(await AddFirewallRule(new firewall.RuleRequest({ serverId, zone, ...rule })));
  }

  if (loading) return <p className="workspace__placeholder">{t('common.loading')}</p>;

  if (status && status.backend === 'none') {
    return (
      <div className="docker-engine">
        <p>
          <TriangleAlert size={13} /> {t('fw.noFirewall')}
        </p>
      </div>
    );
  }

  const editable = status?.editable ?? false;

  return (
    <div className="files-panel">
      <div className="fw-status">
        <span className={`docker-badge ${status?.active ? 'docker-badge--running' : 'docker-badge--stopped'}`}>
          {status?.active ? <ShieldCheck size={12} /> : <ShieldOff size={12} />} {status?.backend}
          {' · '}
          {status?.active ? t('fw.active') : t('fw.inactive')}
        </span>

        {status?.defaultIncoming && (
          <span className="fw-status__item">
            {t('fw.defaultIncoming')}: <strong>{status.defaultIncoming}</strong>
          </span>
        )}
        {/* Zone hanya ada di firewalld. Dibuat bisa dipilih karena aturan
            yang mendarat di zone keliru tidak berlaku pada interface yang
            dimaksud — dan itu tidak terlihat dari daftar aturan saja. */}
        {status?.zone && (
          <span className="fw-status__item">
            {t('fw.zone')}:{' '}
            {status.zones?.length ? (
              <select value={zone || status.zone} onChange={(e) => setZone(e.target.value)}>
                {status.zones.map((z) => (
                  <option key={z} value={z}>
                    {z}
                  </option>
                ))}
              </select>
            ) : (
              <strong>{status.zone}</strong>
            )}
          </span>
        )}
        {/* Daftar yang terpasang ditampilkan supaya jelas KENAPA satu backend
            dipilih — di server yang punya ufw sekaligus firewalld, ini yang
            menjelaskan pilihan aplikasi. */}
        {status && status.installed?.length > 1 && (
          <span className="fw-status__item">
            {t('fw.installed')}: {status.installed.join(', ')}
          </span>
        )}
      </div>

      {!editable && (
        <p className="fw-notice">
          <TriangleAlert size={13} /> {t('fw.readOnly', { backend: status?.backend ?? '' })}
        </p>
      )}

      {/* Peringatan paling penting di halaman ini: menyalakan firewall tanpa
          aturan SSH akan memutus satu-satunya jalan masuk ke server. */}
      {editable && !status?.active && !status?.sshAllowed && (
        <p className="fw-notice fw-notice--warn">
          <Lock size={13} /> {t('fw.sshWarning', { port: String(status?.sshPort ?? 22) })}
        </p>
      )}

      {editable && status && !status.ownerDetectable && (
        <p className="fw-notice">{t('fw.ownerUnknown')}</p>
      )}

      <div className="files-panel__toolbar">
        <button className="btn btn--sm" disabled={refreshing} onClick={() => void load()}>
          {refreshing ? <span className="spinner" /> : <RotateCw size={13} />} {t('common.refresh')}
        </button>

        {editable && (
          <>
            <button className="btn btn--sm" disabled={busy} onClick={() => setShowAdd(true)}>
              <Plus size={13} /> {t('fw.addRule')}
            </button>
            <button
              className={`btn btn--sm${status?.active ? '' : ' btn--primary'}`}
              disabled={busy}
              onClick={() => void handleToggleEnabled()}
            >
              {busy ? <span className="spinner" /> : status?.active ? <ShieldOff size={13} /> : <ShieldCheck size={13} />}{' '}
              {status?.active ? t('fw.disable') : t('fw.enable')}
            </button>
          </>
        )}
      </div>

      {error && <p className="overview__error">{error}</p>}

      <div className="files-panel__table-wrap">
        <table className="files-panel__table">
          <thead>
            <tr>
              <th>{t('fw.colPort')}</th>
              <th>{t('fw.colProtocol')}</th>
              <th>{t('fw.colSource')}</th>
              <th>{t('fw.colAction')}</th>
              <th>{t('fw.colComment')}</th>
              <th className="files-panel__col-actions" />
            </tr>
          </thead>
          <tbody>
            {rules.map((r, i) => (
              <tr key={`${r.id || r.raw}-${i}`}>
                <td className="files-panel__name">
                  {/* Aturan berbentuk service firewalld: portnya ada di
                      servicePorts, bukan port. */}
                  {r.service ? (
                    <>
                      {r.servicePorts || '—'}
                      <span className="fw-tag fw-tag--owner">{r.service}</span>
                    </>
                  ) : (
                    r.port || <span style={{ opacity: 0.55 }}>{r.raw}</span>
                  )}
                  {r.ipv6 && <span className="fw-tag">v6</span>}
                </td>
                <td>{r.protocol || '—'}</td>
                <td>
                  <div className="files-panel__ellipsis" title={r.source || t('fw.sourceAnyLabel')}>
                    {r.source || t('fw.sourceAnyLabel')}
                  </div>
                </td>
                <td>
                  <span className={`docker-badge ${r.action === 'allow' ? 'docker-badge--running' : 'docker-badge--stopped'}`}>
                    {r.action === 'allow' ? t('fw.allow') : t('fw.deny')}
                  </span>
                </td>
                <td>
                  <div className="files-panel__ellipsis" title={r.comment}>
                    {r.owner && <span className="fw-tag fw-tag--owner">{r.owner}</span>}
                    {r.comment || (r.owner ? '' : '—')}
                  </div>
                </td>
                <td className="files-panel__row-actions">
                  <button
                    title={r.id ? t('fw.deleteRule') : t('fw.notDeletable')}
                    disabled={busy || !editable || !r.id}
                    onClick={() => void handleDelete(r)}
                  >
                    <Trash2 size={14} />
                  </button>
                </td>
              </tr>
            ))}
            {rules.length === 0 && (
              <tr>
                <td colSpan={6} className="files-panel__empty">
                  {t('fw.noRules')}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {showAdd && <FirewallRuleModal onConfirm={handleAdd} onClose={() => setShowAdd(false)} />}
    </div>
  );
}

function ruleLabel(r: firewall.Rule) {
  const parts = [r.action, r.port || r.raw];
  if (r.protocol) parts.push(`/${r.protocol}`);
  if (r.source) parts.push(`from ${r.source}`);
  return parts.join(' ');
}
