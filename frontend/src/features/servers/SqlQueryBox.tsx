import CodeMirror from '@uiw/react-codemirror';
import { EditorView, keymap } from '@codemirror/view';
import { Prec } from '@codemirror/state';
import { loadLanguage } from '@uiw/codemirror-extensions-langs';
import { useThemeStore } from '../../store/theme';
import { ChevronLeft, ChevronRight, Play } from 'lucide-react';

const sqlLang = loadLanguage('sql');

// Kotak query ala Navicat: editor SQL sungguhan (CodeMirror — sudah dipakai
// EditFileModal, jadi tidak menambah dependensi) dengan syntax highlighting,
// nomor baris, dan Ctrl/Cmd+Enter untuk menjalankan — bukan <textarea>
// polos seperti sebelumnya.
//
// Hasilnya dipaginasi: hanya satu halaman yang ditarik dari server per
// eksekusi (lihat paginateSelectSQL di backend). Navigasinya sengaja
// "Sebelumnya/Berikutnya" tanpa total halaman — menghitung total butuh
// COUNT(*) atas seluruh hasil query, yang untuk tabel besar justru jadi
// bagian paling lambat. hasMore didapat gratis dari baris ke-(limit+1).
export function SqlQueryBox({
  value,
  onChange,
  onRun,
  busy,
  pageSize,
  offset,
  rowCount,
  paginated,
  hasMore,
  database,
}: {
  value: string;
  onChange: (v: string) => void;
  onRun: (offset: number) => void;
  busy: boolean;
  pageSize: number;
  offset: number;
  rowCount: number;
  paginated: boolean;
  hasMore: boolean;
  database: string;
}) {
  // Editor ikut tema aplikasi — dulu theme="dark" dipaku, jadi kotak query
  // tetap gelap total saat UI dalam tema terang.
  const editorTheme = useThemeStore((s) => s.mode);

  // Prec.highest supaya Ctrl+Enter tidak keburu ditangkap keymap bawaan
  // CodeMirror (di sana Enter/Ctrl+Enter punya arti sendiri).
  const runKeymap = Prec.highest(
    keymap.of([
      {
        key: 'Mod-Enter',
        run: () => {
          if (!busy && value.trim()) onRun(0);
          return true;
        },
      },
    ]),
  );

  return (
    <div className="sql-query-box">
      <CodeMirror
        value={value}
        theme={editorTheme}
        height="120px"
        placeholder={`Query bebas di database "${database}" — satu statement. Ctrl+Enter untuk menjalankan.`}
        extensions={[runKeymap, EditorView.lineWrapping, ...(sqlLang ? [sqlLang] : [])]}
        onChange={onChange}
      />
      <div className="db-explorer__toolbar">
        <button className="btn btn--sm btn--primary" disabled={busy || !value.trim()} onClick={() => onRun(0)}>
          {busy ? <span className="spinner" /> : <Play size={13} />} {busy ? 'Menjalankan…' : 'Jalankan'}
        </button>
        <span style={{ fontSize: 11, opacity: 0.5 }}>Ctrl+Enter</span>

        {paginated && (
          <div className="db-explorer__pagination" style={{ marginLeft: 'auto' }}>
            <button className="btn btn--sm" disabled={busy || offset <= 0} onClick={() => onRun(Math.max(0, offset - pageSize))}>
              <ChevronLeft size={13} /> Sebelumnya
            </button>
            <span>
              {rowCount === 0 ? 'Tidak ada baris' : `Baris ${offset + 1}–${offset + rowCount}`}
            </span>
            <button className="btn btn--sm" disabled={busy || !hasMore} onClick={() => onRun(offset + pageSize)}>
              Berikutnya <ChevronRight size={13} />
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
