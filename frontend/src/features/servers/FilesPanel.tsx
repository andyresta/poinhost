import { useEffect, useState, useCallback } from 'react';
import {
  ListFiles,
  CreateFolder,
  CreateFile,
  RenameFile,
  DeleteFiles,
  CompressFiles,
  ExtractArchive,
  UploadFilesToServer,
  DownloadFileFromServer,
} from '../../../wailsjs/go/main/App';
import { files, sshpool } from '../../../wailsjs/go/models';
import { PromptModal } from './PromptModal';
import { CompressModal } from './CompressModal';

const ARCHIVE_RE = /\.(zip|tar\.gz|tgz)$/i;

function joinPath(dir: string, name: string): string {
  return dir === '/' ? `/${name}` : `${dir}/${name}`;
}

function formatSize(bytes: number, isDir: boolean): string {
  if (isDir) return '—';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`;
}

function formatDate(modTime: string): string {
  const d = new Date(modTime);
  return Number.isNaN(d.getTime()) ? modTime : d.toLocaleString();
}

function breadcrumbParts(p: string): { label: string; path: string }[] {
  if (p === '/') return [{ label: '/', path: '/' }];
  const segs = p.split('/').filter(Boolean);
  const parts = [{ label: '/', path: '/' }];
  let acc = '';
  for (const seg of segs) {
    acc += '/' + seg;
    parts.push({ label: seg, path: acc });
  }
  return parts;
}

// File manager remote via SFTP untuk SATU tab. Sama seperti Overview &
// Terminal, komponen ini di-keep-alive (hidden, bukan unmount) selama tab
// masih terbuka — lihat ServerWorkspace.tsx — jadi direktori & seleksi yang
// sedang dibuka tidak hilang saat user pindah ke modul lain lalu balik lagi.
export function FilesPanel({ serverId }: { serverId: string }) {
  const [path, setPath] = useState('/');
  const [result, setResult] = useState<files.ListResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [modal, setModal] = useState<'newFolder' | 'newFile' | 'compress' | null>(null);
  const [renameTarget, setRenameTarget] = useState<sshpool.FileEntry | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(
    async (targetPath: string) => {
      setLoading(true);
      setError(null);
      try {
        const res = await ListFiles(serverId, targetPath);
        setResult(res);
        setPath(res.path);
        setSelected(new Set());
      } catch (e) {
        setError(String(e));
      } finally {
        setLoading(false);
      }
    },
    [serverId],
  );

  useEffect(() => {
    void load('/');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId]);

  const entries = result?.entries ?? [];
  const selectedEntries = entries.filter((e) => selected.has(e.path));
  const singleSelected = selectedEntries.length === 1 ? selectedEntries[0] : null;

  function toggleSelect(p: string) {
    setSelected((s) => {
      const next = new Set(s);
      if (next.has(p)) next.delete(p);
      else next.add(p);
      return next;
    });
  }

  function openEntry(entry: sshpool.FileEntry) {
    if (entry.isDir) void load(entry.path);
  }

  async function withBusy(fn: () => Promise<void>) {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleUpload() {
    await withBusy(async () => {
      await UploadFilesToServer(serverId, path);
      await load(path);
    });
  }

  async function handleDownload(entry: sshpool.FileEntry) {
    await withBusy(async () => {
      await DownloadFileFromServer(serverId, entry.path);
    });
  }

  async function handleDelete() {
    if (selected.size === 0) return;
    if (!confirm(`Hapus ${selected.size} item terpilih? Tindakan ini tidak bisa dibatalkan.`)) return;
    await withBusy(async () => {
      await DeleteFiles(new files.DeleteRequest({ serverId, paths: [...selected] }));
      await load(path);
    });
  }

  async function handleExtract(entry: sshpool.FileEntry) {
    await withBusy(async () => {
      await ExtractArchive(new files.ExtractRequest({ serverId, archivePath: entry.path, destPath: path }));
      await load(path);
    });
  }

  return (
    <div className="files-panel">
      <div className="files-panel__toolbar">
        <button className="btn btn--sm" disabled={path === '/' || loading} onClick={() => void load(result?.parent ?? '/')}>
          ⬆ Naik
        </button>
        <button className="btn btn--sm" disabled={busy} onClick={() => setModal('newFolder')}>
          + Folder
        </button>
        <button className="btn btn--sm" disabled={busy} onClick={() => setModal('newFile')}>
          + File
        </button>
        <button className="btn btn--sm" disabled={busy} onClick={() => void handleUpload()}>
          ⬆ Upload
        </button>
        <button className="btn btn--sm" disabled={loading} onClick={() => void load(path)}>
          ⟲
        </button>

        {selected.size > 0 && (
          <div className="files-panel__bulk">
            <span>{selected.size} dipilih</span>
            {singleSelected && !singleSelected.isDir && (
              <button className="btn btn--sm" onClick={() => void handleDownload(singleSelected)}>
                Download
              </button>
            )}
            {singleSelected && (
              <button className="btn btn--sm" onClick={() => setRenameTarget(singleSelected)}>
                Rename
              </button>
            )}
            <button className="btn btn--sm" onClick={() => setModal('compress')}>
              Kompres
            </button>
            <button className="btn btn--sm btn--danger" onClick={() => void handleDelete()}>
              Hapus
            </button>
          </div>
        )}
      </div>

      <div className="files-panel__breadcrumb">
        {breadcrumbParts(path).map((part, i) => (
          <span key={part.path}>
            {i > 0 && <span className="files-panel__breadcrumb-sep">/</span>}
            <button className="files-panel__breadcrumb-item" onClick={() => void load(part.path)}>
              {part.label}
            </button>
          </span>
        ))}
      </div>

      {error && <p className="overview__error">{error}</p>}

      <div className="files-panel__table-wrap">
        <table className="files-panel__table">
          <thead>
            <tr>
              <th className="files-panel__col-check" />
              <th>Nama</th>
              <th>Ukuran</th>
              <th>Diubah</th>
              <th className="files-panel__col-actions" />
            </tr>
          </thead>
          <tbody>
            {entries.map((entry) => (
              <tr key={entry.path} className={selected.has(entry.path) ? 'files-panel__row--selected' : ''}>
                <td>
                  <input type="checkbox" checked={selected.has(entry.path)} onChange={() => toggleSelect(entry.path)} />
                </td>
                <td className="files-panel__name" onDoubleClick={() => openEntry(entry)}>
                  <span className="files-panel__icon">{entry.isDir ? '📁' : '📄'}</span>
                  {entry.name}
                </td>
                <td>{formatSize(entry.size, entry.isDir)}</td>
                <td>{formatDate(entry.modTime)}</td>
                <td className="files-panel__row-actions">
                  {!entry.isDir && (
                    <button title="Download" onClick={() => void handleDownload(entry)}>
                      ⬇
                    </button>
                  )}
                  {!entry.isDir && ARCHIVE_RE.test(entry.name) && (
                    <button title="Ekstrak di sini" onClick={() => void handleExtract(entry)}>
                      📦
                    </button>
                  )}
                  <button title="Rename" onClick={() => setRenameTarget(entry)}>
                    ✎
                  </button>
                </td>
              </tr>
            ))}
            {!loading && entries.length === 0 && (
              <tr>
                <td colSpan={5} className="files-panel__empty">
                  Direktori kosong.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        {loading && <div className="files-panel__loading">Memuat…</div>}
      </div>

      {modal === 'newFolder' && (
        <PromptModal
          title="Folder baru"
          label="Nama folder"
          confirmLabel="Buat"
          onClose={() => setModal(null)}
          onConfirm={async (name) => {
            await CreateFolder(new files.MkdirRequest({ serverId, path, name }));
            await load(path);
          }}
        />
      )}

      {modal === 'newFile' && (
        <PromptModal
          title="File baru"
          label="Nama file"
          confirmLabel="Buat"
          onClose={() => setModal(null)}
          onConfirm={async (name) => {
            await CreateFile(new files.CreateFileRequest({ serverId, path, name }));
            await load(path);
          }}
        />
      )}

      {modal === 'compress' && (
        <CompressModal
          defaultName={selectedEntries.length === 1 ? selectedEntries[0].name : 'archive'}
          onClose={() => setModal(null)}
          onConfirm={async (name, format) => {
            const ext = format === 'zip' ? '.zip' : '.tar.gz';
            const archiveName = name.endsWith(ext) ? name : name + ext;
            await CompressFiles(
              new files.CompressRequest({
                serverId,
                sources: [...selected],
                archivePath: joinPath(path, archiveName),
                format,
              }),
            );
            await load(path);
          }}
        />
      )}

      {renameTarget && (
        <PromptModal
          title="Rename"
          label="Nama baru"
          initialValue={renameTarget.name}
          confirmLabel="Rename"
          onClose={() => setRenameTarget(null)}
          onConfirm={async (newName) => {
            await RenameFile(
              new files.RenameRequest({
                serverId,
                oldPath: renameTarget.path,
                newPath: joinPath(path, newName),
              }),
            );
            await load(path);
          }}
        />
      )}
    </div>
  );
}
