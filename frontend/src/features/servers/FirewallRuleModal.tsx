import { useState } from 'react';
import { X } from 'lucide-react';
import { useT } from '../../i18n';

// Modal "Tambah Aturan" — form dipisah ke modal, bukan ditempel di bawah
// tabel, mengikuti pola form lain di aplikasi ini.
//
// Pilihan nilainya sengaja berupa dropdown/preset, bukan teks bebas, karena
// backend memvalidasinya dengan whitelist ketat: apa pun di luar bentuk yang
// diizinkan akan ditolak, jadi lebih baik user tidak bisa mengetiknya sejak
// awal daripada baru tahu setelah menekan Simpan.
export function FirewallRuleModal({
  onConfirm,
  onClose,
}: {
  onConfirm: (rule: {
    port: string;
    protocol: string;
    source: string;
    action: string;
    comment: string;
  }) => Promise<void>;
  onClose: () => void;
}) {
  const t = useT();
  const [port, setPort] = useState('');
  const [protocol, setProtocol] = useState('tcp');
  const [source, setSource] = useState('');
  const [action, setAction] = useState('allow');
  const [comment, setComment] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleConfirm() {
    if (!port.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await onConfirm({ port: port.trim(), protocol, source: source.trim(), action, comment: comment.trim() });
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--small">
        <div className="modal-card__header">
          <h2>{t('fw.addRule')}</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>

        <div className="modal-card__body">
          <label className="field">
            <span>{t('fw.port')}</span>
            <input
              value={port}
              onChange={(e) => setPort(e.target.value)}
              placeholder="80"
              autoFocus
            />
            <small className="modal-card__hint">{t('fw.portHint')}</small>
          </label>

          <label className="field">
            <span>{t('fw.protocol')}</span>
            <select value={protocol} onChange={(e) => setProtocol(e.target.value)}>
              <option value="tcp">TCP</option>
              <option value="udp">UDP</option>
            </select>
          </label>

          <label className="field">
            <span>{t('fw.action')}</span>
            <select value={action} onChange={(e) => setAction(e.target.value)}>
              <option value="allow">{t('fw.allow')}</option>
              <option value="deny">{t('fw.deny')}</option>
            </select>
          </label>

          <label className="field">
            <span>{t('fw.source')}</span>
            <input
              value={source}
              onChange={(e) => setSource(e.target.value)}
              placeholder={t('fw.sourceAny')}
            />
            <small className="modal-card__hint">{t('fw.sourceHint')}</small>
          </label>

          <label className="field">
            <span>{t('fw.comment')}</span>
            <input value={comment} onChange={(e) => setComment(e.target.value)} />
          </label>

          {error && <p className="overview__error">{error}</p>}
        </div>

        <div className="modal-card__footer">
          <button className="btn" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </button>
          <button className="btn btn--primary" onClick={() => void handleConfirm()} disabled={busy || !port.trim()}>
            {busy ? <span className="spinner" /> : null} {t('common.save')}
          </button>
        </div>
      </div>
    </div>
  );
}
