import { useState } from 'react';
import { Folder, FileText, X } from 'lucide-react';
import { SearchFiles } from '../../../wailsjs/go/main/App';
import { files } from '../../../wailsjs/go/models';

// Pencarian file/direktori rekursif dari direktori yang sedang dibuka.
// Hasil ditampilkan sebagai daftar terpisah (bukan menimpa tabel utama)
// karena hit-nya bisa datang dari sub-direktori mana pun — klik salah satu
// hasil pindah FilesPanel ke direktori tempat file itu berada.
export function SearchModal({
  serverId,
  path,
  asUser,
  onOpenParent,
  onClose,
}: {
  serverId: string;
  path: string;
  asUser: string;
  onOpenParent: (parentPath: string) => void;
  onClose: () => void;
}) {
  const [query, setQuery] = useState('');
  const [result, setResult] = useState<files.SearchResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSearch() {
    if (!query.trim()) return;
    setLoading(true);
    setError(null);
    try {
      const res = await SearchFiles(new files.SearchRequest({ serverId, path, query: query.trim(), asUser }));
      setResult(res);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }

  function parentOf(hitPath: string): string {
    const idx = hitPath.lastIndexOf('/');
    if (idx <= 0) return '/';
    return hitPath.slice(0, idx);
  }

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card">
        <div className="modal-card__header">
          <h2>Cari File</h2>
          <button className="modal-card__close" onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        <div className="modal-card__body">
          <p className="chmod-path">Mencari di dalam: {path}</p>
          <div className="search-input-row">
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && void handleSearch()}
              placeholder="mis. nginx.conf, .env, backup"
            />
            <button className="btn btn--primary btn--sm" disabled={loading || !query.trim()} onClick={() => void handleSearch()}>
              {loading && <span className="spinner" />} {loading ? 'Mencari…' : 'Cari'}
            </button>
          </div>

          {error && <p className="overview__error">{error}</p>}

          {result && (
            <ul className="search-results">
              {result.hits.length === 0 && <li className="files-panel__empty">Tidak ada hasil untuk "{result.query}".</li>}
              {result.hits.map((hit) => (
                <li key={hit.path} className="search-result-item">
                  <span>{hit.isDir ? <Folder size={14} /> : <FileText size={14} />}</span>
                  <span className="search-result-path">{hit.path}</span>
                  <button
                    className="btn btn--sm"
                    onClick={() => {
                      onOpenParent(hit.isDir ? hit.path : parentOf(hit.path));
                      onClose();
                    }}
                  >
                    Buka
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
        <div className="modal-card__footer">
          <button className="btn btn--ghost" onClick={onClose}>
            Tutup
          </button>
        </div>
      </div>
    </div>
  );
}
