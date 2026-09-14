import { useState } from 'react';
import { X } from 'lucide-react';

// Modal kecil "Tambah Subdomain" — parent ditampilkan read-only, user cuma
// mengisi label-nya (mis. "app" untuk app.contoh.com).
export function SubdomainModal({
  parent,
  onConfirm,
  onClose,
}: {
  parent: string;
  onConfirm: (label: string) => void | Promise<void>;
  onClose: () => void;
}) {
  const [label, setLabel] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleConfirm() {
    if (!label.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await onConfirm(label.trim());
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
          <h2>Tambah Subdomain</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body">
          <label className="form-field">
            <span>Subdomain</span>
            <div className="subdomain-input">
              <input
                autoFocus
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && void handleConfirm()}
                placeholder="app"
              />
              <span>.{parent}</span>
            </div>
          </label>
          {error && <p className="overview__error">{error}</p>}
        </div>
        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={onClose}>
            Batal
          </button>
          <button className="btn btn--primary" disabled={busy || !label.trim()} onClick={() => void handleConfirm()}>
            {busy && <span className="spinner" />} {busy ? 'Membuat…' : 'Buat'}
          </button>
        </div>
      </div>
    </div>
  );
}
