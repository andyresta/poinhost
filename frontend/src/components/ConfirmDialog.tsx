import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react';
import { useT } from '../i18n';

export interface ConfirmOptions {
  title?: string;
  message: ReactNode;
  /** Teks tambahan kecil di bawah pesan utama (konsekuensi, catatan teknis). */
  detail?: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  /** true = aksi merusak: tombol konfirmasi jadi merah. */
  danger?: boolean;
}

type ConfirmFn = (opts: ConfirmOptions | string) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn | null>(null);

// useConfirm mengganti window.confirm bawaan browser. Selain tampilannya
// yang tidak mengikuti tema aplikasi sama sekali, confirm() bawaan itu
// MEMBLOKIR seluruh proses render WebView selama dialognya terbuka — di
// Wails efeknya jendela terasa "beku". Bentuk pemakaiannya sengaja dibuat
// mirip aslinya supaya pemanggil cukup menambah await:
//
//   if (!(await confirm('Hapus ini?'))) return;
export function useConfirm(): ConfirmFn {
  const ctx = useContext(ConfirmContext);
  if (!ctx) throw new Error('useConfirm dipakai di luar <ConfirmProvider>');
  return ctx;
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const t = useT();
  const [opts, setOpts] = useState<ConfirmOptions | null>(null);
  // Promise-nya diselesaikan lewat ref supaya identitas fungsi confirm()
  // tetap stabil (tidak memicu render ulang pemanggil setiap dialog buka).
  const resolveRef = useRef<((v: boolean) => void) | null>(null);

  const confirm = useCallback<ConfirmFn>((raw) => {
    setOpts(typeof raw === 'string' ? { message: raw } : raw);
    return new Promise<boolean>((resolve) => {
      resolveRef.current = resolve;
    });
  }, []);

  function settle(value: boolean) {
    setOpts(null);
    resolveRef.current?.(value);
    resolveRef.current = null;
  }

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {opts && (
        <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && settle(false)}>
          <div className="modal-card modal-card--small" role="alertdialog" aria-modal="true">
            <div className="modal-card__header">
              <h2>{opts.title ?? 'Konfirmasi'}</h2>
            </div>
            <div className="modal-card__body">
              <p style={{ margin: 0 }}>{opts.message}</p>
              {opts.detail && <p className="chmod-path" style={{ margin: 0 }}>{opts.detail}</p>}
            </div>
            <div className="modal-card__footer">
              <button className="btn btn--ghost" onClick={() => settle(false)}>
                {opts.cancelLabel ?? t('common.cancel')}
              </button>
              <button
                className={`btn ${opts.danger ? 'btn--danger' : 'btn--primary'}`}
                autoFocus
                onClick={() => settle(true)}
              >
                {opts.confirmLabel ?? t('common.continue')}
              </button>
            </div>
          </div>
        </div>
      )}
    </ConfirmContext.Provider>
  );
}
