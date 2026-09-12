import { useState } from 'react';

// Modal input teks kecil generik (nama folder/file baru, rename, dst) —
// dipakai berkali-kali di FilesPanel supaya tidak perlu `window.prompt()`
// (dukungannya tidak konsisten lintas platform WebView) sekaligus konsisten
// gaya dengan modal lain di app (ServerFormModal, confirm delete).
export function PromptModal({
  title,
  label,
  initialValue = '',
  confirmLabel = 'Simpan',
  onConfirm,
  onClose,
}: {
  title: string;
  label: string;
  initialValue?: string;
  confirmLabel?: string;
  onConfirm: (value: string) => void | Promise<void>;
  onClose: () => void;
}) {
  const [value, setValue] = useState(initialValue);
  const [busy, setBusy] = useState(false);

  async function handleConfirm() {
    if (!value.trim()) return;
    setBusy(true);
    try {
      await onConfirm(value.trim());
      onClose();
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--small">
        <div className="modal-card__header">
          <h2>{title}</h2>
          <button className="modal-card__close" onClick={onClose}>
            ×
          </button>
        </div>
        <div className="modal-card__body">
          <label className="form-field">
            <span>{label}</span>
            <input
              autoFocus
              value={value}
              onChange={(e) => setValue(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && void handleConfirm()}
            />
          </label>
        </div>
        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={onClose}>
            Batal
          </button>
          <button className="btn btn--primary" disabled={busy || !value.trim()} onClick={() => void handleConfirm()}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
