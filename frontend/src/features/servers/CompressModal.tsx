import { useState } from 'react';
import { X } from 'lucide-react';

// Modal kecil khusus kompresi: nama arsip + format (zip / tar.gz). Terpisah
// dari PromptModal karena butuh satu field tambahan (segmented format).
export function CompressModal({
  defaultName,
  onConfirm,
  onClose,
}: {
  defaultName: string;
  onConfirm: (name: string, format: 'zip' | 'tar.gz') => void | Promise<void>;
  onClose: () => void;
}) {
  const [name, setName] = useState(defaultName);
  const [format, setFormat] = useState<'zip' | 'tar.gz'>('zip');
  const [busy, setBusy] = useState(false);

  async function handleConfirm() {
    if (!name.trim()) return;
    setBusy(true);
    try {
      await onConfirm(name.trim(), format);
      onClose();
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--small">
        <div className="modal-card__header">
          <h2>Kompres (gzip)</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body">
          <label className="form-field">
            <span>Nama arsip</span>
            <input autoFocus value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <div className="form-field">
            <span>Format</span>
            <div className="segmented">
              <button
                className={`segmented__item${format === 'zip' ? ' segmented__item--active' : ''}`}
                onClick={() => setFormat('zip')}
              >
                .zip
              </button>
              <button
                className={`segmented__item${format === 'tar.gz' ? ' segmented__item--active' : ''}`}
                onClick={() => setFormat('tar.gz')}
              >
                .tar.gz
              </button>
            </div>
          </div>
        </div>
        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={onClose}>
            Batal
          </button>
          <button className="btn btn--primary" disabled={busy || !name.trim()} onClick={() => void handleConfirm()}>
            {busy && <span className="spinner" />} Kompres
          </button>
        </div>
      </div>
    </div>
  );
}
