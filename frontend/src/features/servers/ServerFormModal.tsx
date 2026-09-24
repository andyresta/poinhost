import { useState } from 'react';
import { X } from 'lucide-react';
import { TestServerConnection, TrustServerHostKey } from '../../../wailsjs/go/main/App';
import { servers } from '../../../wailsjs/go/models';
import { useTabsStore } from '../../store/tabs';
import { useT } from '../../i18n';

const COLOR_SWATCHES = [
  'var(--accent)', '#ec4899', '#22c55e', '#f59e0b',
  '#06b6d4', '#a855f7', '#ef4444', '#64748b',
];

type TestState = 'idle' | 'testing' | 'ok' | 'unknown' | 'mismatch' | 'error';

function toRequest(server?: servers.Server): servers.SaveServerRequest {
  return new servers.SaveServerRequest({
    id: server?.id,
    name: server?.name ?? '',
    host: server?.host ?? '',
    port: server?.port ?? 22,
    username: server?.username ?? '',
    authType: server?.authType ?? 'key',
    keyPath: server?.keyPath ?? '',
    password: '',
    tags: server?.tags ?? [],
    color: server?.color ?? COLOR_SWATCHES[0],
    notes: server?.notes ?? '',
    useSudo: server?.useSudo ?? false,
  });
}

// Modal tambah/edit server. Berbeda dari homepoin (satu form panjang, Simpan
// selalu tersembunyi sampai tes lolos, bahkan untuk edit kecil seperti ganti
// warna): di sini form dikelompokkan per section supaya lebih mudah dipindai,
// dan Simpan HANYA digembok di belakang tes koneksi untuk server BARU — edit
// server yang sudah ada & terbukti jalan tidak perlu dites ulang tiap ganti
// nama/warna/catatan (lihat canSave di bawah).
export function ServerFormModal({
  mode,
  initial,
  onClose,
}: {
  mode: 'create' | 'edit';
  initial?: servers.Server;
  onClose: () => void;
}) {
  const saveServer = useTabsStore((s) => s.saveServer);
  const t = useT();
  const [form, setForm] = useState<servers.SaveServerRequest>(() => toRequest(initial));
  const [tagInput, setTagInput] = useState('');
  const [testState, setTestState] = useState<TestState>('idle');
  const [testDetail, setTestDetail] = useState<{
    latency?: string;
    fingerprint?: string;
    oldFingerprint?: string;
    message?: string;
  }>({});
  const [busy, setBusy] = useState(false);

  const canSave = mode === 'edit' || testState === 'ok';

  function update<K extends keyof servers.SaveServerRequest>(key: K, value: servers.SaveServerRequest[K]) {
    setForm((f) => ({ ...f, [key]: value }));
    // Field koneksi berubah setelah tes lolos -> tes jadi tidak valid lagi,
    // jangan biarkan Save tetap "hijau" untuk kredensial yang belum dites.
    if (['host', 'port', 'username', 'authType', 'keyPath', 'password'].includes(key as string)) {
      setTestState('idle');
    }
  }

  function addTag() {
    const t = tagInput.trim();
    if (t && !form.tags.includes(t)) {
      update('tags', [...form.tags, t]);
    }
    setTagInput('');
  }

  function removeTag(t: string) {
    update('tags', form.tags.filter((x) => x !== t));
  }

  async function handleTest() {
    setTestState('testing');
    setBusy(true);
    try {
      const res = await TestServerConnection(form);
      setTestDetail({
        latency: res.latency,
        fingerprint: res.fingerprint,
        oldFingerprint: res.oldFingerprint,
        message: res.message,
      });
      setTestState(res.status as TestState);
    } catch (e) {
      setTestDetail({ message: String(e) });
      setTestState('error');
    } finally {
      setBusy(false);
    }
  }

  async function handleTrust() {
    // Fingerprint yang dikirim balik adalah persis yang ditampilkan di layar
    // dan disetujui user. Backend menolak menyimpan host key lain -- lihat
    // sshpool.Pool.TrustHostKey.
    const approved = testDetail.fingerprint;
    if (!approved) {
      setTestDetail({ message: t('srv.trustNeedsTest') });
      setTestState('error');
      return;
    }
    setBusy(true);
    try {
      await TrustServerHostKey(form, approved);
      // Setelah host key dipercaya, ulangi tes supaya status benar-benar
      // terverifikasi "ok" (bukan diasumsikan) sebelum Save diaktifkan.
      await handleTest();
    } catch (e) {
      setTestDetail({ message: String(e) });
      setTestState('error');
    } finally {
      setBusy(false);
    }
  }

  async function handleSave() {
    setBusy(true);
    try {
      await saveServer(form);
      onClose();
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card">
        <div className="modal-card__header">
          <h2>{mode === 'create' ? t('srv.addServer') : t('srv.editServer', { name: initial?.name ?? '' })}</h2>
          <button className="modal-card__close" onClick={onClose} aria-label={t('common.close')}>
            <X size={18} />
          </button>
        </div>

        <div className="modal-card__body">
          <section className="form-section">
            <h3>{t('srv.connection')}</h3>
            <div className="form-grid">
              <label className="form-field form-field--span2">
                <span>{t('common.name')}</span>
                <input
                  value={form.name}
                  onChange={(e) => update('name', e.target.value)}
                  placeholder={t('srv.namePlaceholder')}
                />
              </label>
              <label className="form-field form-field--span2">
                <span>{t('srv.host')}</span>
                <input value={form.host} onChange={(e) => update('host', e.target.value)} placeholder="203.0.113.10" />
              </label>
              <label className="form-field">
                <span>{t('srv.port')}</span>
                <input
                  type="number"
                  value={form.port}
                  onChange={(e) => update('port', Number(e.target.value))}
                />
              </label>
              <label className="form-field">
                <span>{t('srv.username')}</span>
                <input value={form.username} onChange={(e) => update('username', e.target.value)} placeholder="root" />
              </label>
            </div>
          </section>

          <section className="form-section">
            <h3>{t('srv.auth')}</h3>
            <div className="segmented">
              <button
                className={`segmented__item${form.authType === 'key' ? ' segmented__item--active' : ''}`}
                onClick={() => update('authType', 'key')}
              >
                SSH Key
              </button>
              <button
                className={`segmented__item${form.authType === 'password' ? ' segmented__item--active' : ''}`}
                onClick={() => update('authType', 'password')}
              >
                Password
              </button>
            </div>

            {form.authType === 'key' ? (
              <label className="form-field">
                <span>{t('srv.privateKeyPath')}</span>
                <input
                  value={form.keyPath}
                  onChange={(e) => update('keyPath', e.target.value)}
                  placeholder={t('srv.keyPathPlaceholder')}
                />
              </label>
            ) : (
              <label className="form-field">
                <span>
                  Password
                  {mode === 'edit' ? t('srv.passwordUnchanged') : ''}
                </span>
                <input
                  type="password"
                  value={form.password}
                  onChange={(e) => update('password', e.target.value)}
                  autoComplete="new-password"
                />
              </label>
            )}

            <label className="form-check">
              <input
                type="checkbox"
                checked={form.useSudo}
                onChange={(e) => update('useSudo', e.target.checked)}
              />
              <span>{t('srv.hasSudo')}</span>
            </label>
          </section>

          <section className="form-section">
            <h3>{t('srv.appearance')}</h3>
            <div className="form-field">
              <span>{t('srv.color')}</span>
              <div className="color-swatches">
                {COLOR_SWATCHES.map((c) => (
                  <button
                    key={c}
                    className={`color-swatch${form.color === c ? ' color-swatch--active' : ''}`}
                    style={{ backgroundColor: c }}
                    onClick={() => update('color', c)}
                    aria-label={c}
                  />
                ))}
              </div>
            </div>

            <div className="form-field">
              <span>{t('srv.tags')}</span>
              <div className="tag-input">
                {/* Parameter map dinamai `tag`, BUKAN `t`: `t` sudah dipakai
                    fungsi terjemahan di scope ini dan akan tertutupi. */}
                {form.tags.map((tag) => (
                  <span key={tag} className="tag-chip">
                    {tag}
                    <button onClick={() => removeTag(tag)} aria-label={t('srv.removeTag', { tag })}>
                      <X size={11} />
                    </button>
                  </span>
                ))}
                <input
                  value={tagInput}
                  onChange={(e) => setTagInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ',') {
                      e.preventDefault();
                      addTag();
                    }
                  }}
                  placeholder={t('srv.tagsPlaceholder')}
                />
              </div>
            </div>

            <label className="form-field">
              <span>{t('srv.notes')}</span>
              <textarea
                value={form.notes}
                onChange={(e) => update('notes', e.target.value)}
                rows={2}
              />
            </label>
          </section>

          <ConnectionStatus
            state={testState}
            detail={testDetail}
            onTrust={() => void handleTrust()}
            busy={busy}
          />
        </div>

        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={() => void handleTest()} disabled={busy}>
            {testState === 'testing' && <span className="spinner" />}{' '}
            {testState === 'testing' ? t('srv.testing') : t('srv.testConnection')}
          </button>
          <button className="btn btn--primary" onClick={() => void handleSave()} disabled={!canSave || busy}>
            {busy && testState !== 'testing' && <span className="spinner" />} {t('common.save')}
          </button>
        </div>
        {!canSave && <p className="modal-card__hint">{t('srv.mustTestFirst')}</p>}
      </div>
    </div>
  );
}

function ConnectionStatus({
  state,
  detail,
  onTrust,
  busy,
}: {
  state: TestState;
  detail: { latency?: string; fingerprint?: string; oldFingerprint?: string; message?: string };
  onTrust: () => void;
  busy: boolean;
}) {
  const t = useT();
  if (state === 'idle') return null;

  if (state === 'testing') {
    return <div className="connection-status connection-status--pending">{t('srv.testingConnection')}</div>;
  }
  if (state === 'ok') {
    return (
      <div className="connection-status connection-status--ok">
        {t('srv.connected')}
        {detail.latency ? ` · ${detail.latency}` : ''}
      </div>
    );
  }
  if (state === 'error') {
    return (
      <div className="connection-status connection-status--error">
        {detail.message ?? t('srv.connectionFailed')}
      </div>
    );
  }

  // unknown | mismatch -> perlu konfirmasi fingerprint host key.
  return (
    <div className="connection-status connection-status--warn">
      <p>{state === 'mismatch' ? t('srv.fingerprintChanged') : t('srv.hostKeyUntrusted')}</p>
      {detail.oldFingerprint && (
        <code className="connection-status__fp">
          {t('srv.fpOld')}: {detail.oldFingerprint}
        </code>
      )}
      {detail.fingerprint && (
        <code className="connection-status__fp">
          {t('srv.fpNew')}: {detail.fingerprint}
        </code>
      )}
      <button className="btn btn--sm" onClick={onTrust} disabled={busy}>
        {busy && <span className="spinner" />} {t('srv.trustHostKey')}
      </button>
    </div>
  );
}
