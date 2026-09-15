import { useEffect, useState } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { EditorView } from '@codemirror/view';
import { langNames, loadLanguage, type LanguageName } from '@uiw/codemirror-extensions-langs';
import { useThemeStore } from '../../store/theme';
import { ReadFileContent, WriteFileContent } from '../../../wailsjs/go/main/App';
import { files } from '../../../wailsjs/go/models';
import { useConfirm } from '../../components/ConfirmDialog';
import { X } from 'lucide-react';

// Ekstensi umum yang tidak match langsung ke nama bahasa CodeMirror tapi
// jelas maksudnya — sisanya (yang benar-benar tidak dikenal) tetap bisa
// diedit sebagai teks polos, cuma tanpa syntax highlighting.
const EXT_ALIASES: Record<string, LanguageName> = {
  conf: 'cfg',
  env: 'sh',
  log: 'text',
  yaml: 'yml',
};

function detectLanguage(filename: string) {
  const ext = (filename.split('.').pop() ?? '').toLowerCase();
  const name = (EXT_ALIASES[ext] ?? ext) as LanguageName;
  if (!langNames.includes(name)) return null;
  return loadLanguage(name);
}

// Editor teks in-app pakai CodeMirror 6 — dipilih karena ringan (tanpa web
// worker seperti Monaco, penting untuk aplikasi desktop yang harus tetap
// terasa instan) dan `@uiw/codemirror-extensions-langs` sudah memetakan
// puluhan bahasa langsung dari EKSTENSI file, jadi highlight otomatis
// tanpa perlu daftar mapping manual per bahasa (lihat detectLanguage).
export function EditFileModal({
  serverId,
  path,
  asUser = '',
  onClose,
}: {
  serverId: string;
  path: string;
  asUser?: string;
  onClose: () => void;
}) {
  const confirm = useConfirm();
  // Ikut tema aplikasi — lihat catatan yang sama di SqlQueryBox.
  const editorTheme = useThemeStore((s) => s.mode);
  const [content, setContent] = useState('');
  const [original, setOriginal] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const name = path.split('/').pop() ?? path;
  const dirty = content !== original;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    ReadFileContent(serverId, path, asUser)
      .then((res) => {
        if (cancelled) return;
        setContent(res.content);
        setOriginal(res.content);
      })
      .catch((e) => !cancelled && setError(String(e)))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [serverId, path, asUser]);

  async function handleClose() {
    if (dirty) {
      const ok = await confirm({
        title: 'Tutup tanpa menyimpan',
        message: 'Ada perubahan yang belum disimpan. Tutup tanpa menyimpan?',
        confirmLabel: 'Tutup tanpa simpan',
        danger: true,
      });
      if (!ok) return;
    }
    onClose();
  }

  async function handleSave() {
    setSaving(true);
    setError(null);
    try {
      await WriteFileContent(new files.WriteFileRequest({ serverId, path, content, asUser }));
      setOriginal(content);
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  }

  const langExt = detectLanguage(name);

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && void handleClose()}>
      <div className="modal-card modal-card--editor">
        <div className="modal-card__header">
          <h2>
            {name}
            {dirty && <span className="editor-dirty-dot" title="Belum disimpan"> ●</span>}
          </h2>
          <button className="modal-card__close" onClick={() => void handleClose()}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body modal-card__body--editor">
          {loading ? (
            <p className="workspace__placeholder">Memuat…</p>
          ) : (
            <CodeMirror
              value={content}
              height="100%"
              theme={editorTheme}
              extensions={[EditorView.lineWrapping, ...(langExt ? [langExt] : [])]}
              onChange={(value) => setContent(value)}
            />
          )}
          {error && <p className="overview__error">{error}</p>}
        </div>
        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={() => void handleClose()}>
            Tutup
          </button>
          <button className="btn btn--primary" disabled={!dirty || saving || loading} onClick={() => void handleSave()}>
            {saving && <span className="spinner" />} {saving ? 'Menyimpan…' : 'Simpan'}
          </button>
        </div>
      </div>
    </div>
  );
}
