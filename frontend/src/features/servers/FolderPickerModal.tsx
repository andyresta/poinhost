import { useCallback, useEffect, useState } from 'react';
import { Folder, ArrowUp, RefreshCw, X, FolderPlus } from 'lucide-react';
import { ListFiles, CreateFolder } from '../../../wailsjs/go/main/App';
import { files } from '../../../wailsjs/go/models';
import { useT } from '../../i18n';

// Pemilih folder tujuan di server, untuk operasi yang sebelumnya menuntut
// user mengetik path lengkap dari ingatan (Copy, dan operasi lain yang
// nanti butuh tujuan).
//
// Sengaja memakai ListFiles milik modul Files — BUKAN listing modul migrasi —
// supaya `asUser` yang sedang aktif di panel ikut terbawa: folder yang
// terlihat di sini adalah folder yang memang bisa dibaca oleh user yang akan
// menjalankan operasinya, bukan daftar yang lebih luas dari yang sebenarnya
// boleh dia sentuh.
//
// Path tetap bisa diketik di kolom bawah: menempel path panjang dari tempat
// lain kadang lebih cepat daripada menelusuri, dan folder yang belum ada
// tidak bisa ditelusuri sama sekali.
export function FolderPickerModal({
  serverId,
  asUser,
  initialPath,
  title,
  confirmLabel,
  onConfirm,
  onClose,
}: {
  serverId: string;
  asUser: string;
  initialPath: string;
  title: string;
  confirmLabel: string;
  onConfirm: (destPath: string) => void | Promise<void>;
  onClose: () => void;
}) {
  const t = useT();
  const [path, setPath] = useState(initialPath || '/');
  const [result, setResult] = useState<files.ListResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState('');

  const load = useCallback(
    async (target: string) => {
      setLoading(true);
      setError(null);
      try {
        const res = await ListFiles(serverId, target, asUser);
        setResult(res);
        setPath(res.path);
      } catch (e) {
        setError(String(e));
      } finally {
        setLoading(false);
      }
    },
    [serverId, asUser],
  );

  useEffect(() => {
    void load(initialPath || '/');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function handleCreate() {
    const name = newName.trim();
    if (!name) return;
    setBusy(true);
    setError(null);
    try {
      await CreateFolder(new files.MkdirRequest({ serverId, path, name, asUser }));
      setNewName('');
      setCreating(false);
      // Masuk ke folder yang baru dibuat: hampir selalu itu yang dimaksud
      // saat seseorang membuatnya di tengah memilih tujuan.
      await load(path === '/' ? `/${name}` : `${path}/${name}`);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleConfirm() {
    const target = path.trim();
    if (!target) return;
    setBusy(true);
    try {
      await onConfirm(target);
      onClose();
    } catch (e) {
      setError(String(e));
      setBusy(false);
      return;
    }
    setBusy(false);
  }

  const dirs = (result?.entries ?? []).filter((e) => e.isDir);
  const crumbs = path.split('/').filter(Boolean);

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card">
        <div className="modal-card__header">
          <h2>{title}</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>

        <div className="modal-card__body">
          <div className="picker__toolbar">
            <button
              className="btn btn--sm"
              title={t('picker.up')}
              disabled={loading || path === '/'}
              onClick={() => void load(result?.parent || '/')}
            >
              <ArrowUp size={13} />
            </button>
            <button
              className="btn btn--sm"
              title={t('common.refresh')}
              disabled={loading}
              onClick={() => void load(path)}
            >
              <RefreshCw size={13} />
            </button>
            <button
              className="btn btn--sm"
              title={t('picker.newFolder')}
              disabled={loading || busy}
              onClick={() => setCreating((v) => !v)}
            >
              <FolderPlus size={13} />
            </button>
            <div className="picker__crumbs">
              <button className="picker__crumb" onClick={() => void load('/')}>
                /
              </button>
              {crumbs.map((part, i) => (
                <span key={i}>
                  <button
                    className="picker__crumb"
                    onClick={() => void load('/' + crumbs.slice(0, i + 1).join('/'))}
                  >
                    {part}
                  </button>
                  <span className="picker__crumb-sep">/</span>
                </span>
              ))}
            </div>
          </div>

          {creating && (
            <div className="picker__newfolder">
              <input
                autoFocus
                value={newName}
                placeholder={t('picker.newFolderName')}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && void handleCreate()}
              />
              <button
                className="btn btn--sm btn--primary"
                disabled={busy || !newName.trim()}
                onClick={() => void handleCreate()}
              >
                {t('common.add')}
              </button>
            </div>
          )}

          <div className="picker__list">
            <div className="picker__scroll">
            {error && <div className="picker__error">{error}</div>}
            {!error &&
              dirs.map((entry) => (
                <button
                  key={entry.path}
                  className="picker__row"
                  onDoubleClick={() => void load(entry.path)}
                  onClick={() => setPath(entry.path)}
                >
                  <Folder size={13} />
                  <span className="picker__name">{entry.name}</span>
                </button>
              ))}
            {!error && !loading && dirs.length === 0 && (
              <div className="picker__empty">{t('picker.noSubfolder')}</div>
            )}
            </div>
            {loading && <div className="picker__loading">{t('common.loading')}</div>}
          </div>

          <label className="form-field">
            <span>{t('picker.destination')}</span>
            <input value={path} spellCheck={false} onChange={(e) => setPath(e.target.value)} />
          </label>
        </div>

        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button
            className="btn btn--primary"
            disabled={busy || !path.trim()}
            onClick={() => void handleConfirm()}
          >
            {busy && <span className="spinner" />} {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
