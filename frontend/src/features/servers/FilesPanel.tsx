import { useEffect, useState, useCallback, useRef } from 'react';
import { ArrowUp, Upload, Search, RotateCw, Folder, FileText, Download, FilePenLine, Package, Lock, Pencil } from 'lucide-react';
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
  ChmodFile,
  CopyFiles,
  ListSystemUsers,
} from '../../../wailsjs/go/main/App';
import { files, sshpool } from '../../../wailsjs/go/models';
import { useTabsStore } from '../../store/tabs';
import { PromptModal } from './PromptModal';
import { CompressModal } from './CompressModal';
import { ChmodModal } from './ChmodModal';
import { EditFileModal } from './EditFileModal';
import { SearchModal } from './SearchModal';

// File > 512KB tidak ditawari untuk diedit sebagai teks — kemungkinan besar
// biner atau terlalu besar untuk nyaman diedit di aplikasi (backend sendiri
// masih menolak sampai batas lebih longgar, 2MB, ini cuma sinyal UI supaya
// tombol Edit tidak muncul untuk file yang jelas bukan konfigurasi/skrip).
const EDITABLE_SIZE_HINT = 512 * 1024;

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

function breadcrumbParts(p: string, root: string): { label: string; path: string }[] {
  const rootLabel = root === '/' ? '/' : (root.split('/').filter(Boolean).pop() ?? root);
  if (p === root) return [{ label: rootLabel, path: root }];
  const rest = p.startsWith(root === '/' ? '/' : root + '/') ? p.slice(root === '/' ? 1 : root.length + 1) : p;
  const segs = rest.split('/').filter(Boolean);
  const parts = [{ label: rootLabel, path: root }];
  let acc = root;
  for (const seg of segs) {
    acc = acc === '/' ? `/${seg}` : `${acc}/${seg}`;
    parts.push({ label: seg, path: acc });
  }
  return parts;
}

// File manager remote via SFTP untuk SATU tab. Sama seperti Overview &
// Terminal, komponen ini di-keep-alive (hidden, bukan unmount) selama tab
// masih terbuka — lihat ServerWorkspace.tsx — jadi direktori & seleksi yang
// sedang dibuka tidak hilang saat user pindah ke modul lain lalu balik lagi.
//
// `rootPath` (opsional) mengunci navigasi tidak bisa naik di atasnya —
// dipakai tab Files milik satu domain di menu Website (dikunci ke document
// root domain itu, lihat DomainFilesTab.tsx), kosongkan untuk modul Files
// biasa (root filesystem penuh, mulai dari "/").
export function FilesPanel({ serverId, rootPath = '/' }: { serverId: string; rootPath?: string }) {
  const server = useTabsStore((s) => s.servers.find((x) => x.id === serverId));

  const [path, setPath] = useState(rootPath);
  const [result, setResult] = useState<files.ListResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [modal, setModal] = useState<'newFolder' | 'newFile' | 'compress' | 'copy' | 'search' | null>(
    null,
  );
  const [renameTarget, setRenameTarget] = useState<sshpool.FileEntry | null>(null);
  const [editTarget, setEditTarget] = useState<sshpool.FileEntry | null>(null);
  const [chmodTarget, setChmodTarget] = useState<sshpool.FileEntry | null>(null);
  const [busy, setBusy] = useState(false);

  // "Jalankan sebagai" — kosong berarti user SSH biasa (jalur SFTP cepat,
  // tanpa sudo sama sekali). Cuma relevan kalau server ini useSudo=true;
  // lihat internal/modules/files/access.go untuk validasi & elevasi sudo
  // sesungguhnya di backend (dropdown ini cuma UI-nya).
  const [asUser, setAsUser] = useState('');
  const [systemUsers, setSystemUsers] = useState<files.SystemUser[]>([]);

  const load = useCallback(
    async (targetPath: string) => {
      setLoading(true);
      setError(null);
      try {
        const res = await ListFiles(serverId, targetPath, asUser);
        setResult(res);
        setPath(res.path);
        setSelected(new Set());
      } catch (e) {
        setError(String(e));
      } finally {
        setLoading(false);
      }
    },
    [serverId, asUser],
  );

  useEffect(() => {
    void load(rootPath);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, rootPath]);

  // Ganti "jalankan sebagai" -> muat ulang direktori yang sama sebagai user
  // baru (bisa jadi kelihatan berbeda isinya kalau permission direktori
  // membatasi siapa yang boleh lihat apa). Dilewati saat mount pertama —
  // efek di atas sudah menangani load awal, jadi tidak fetch dobel.
  const mountedRef = useRef(false);
  useEffect(() => {
    if (!mountedRef.current) {
      mountedRef.current = true;
      return;
    }
    void load(path);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [asUser]);

  useEffect(() => {
    if (!server?.useSudo) {
      setSystemUsers([]);
      return;
    }
    ListSystemUsers(serverId)
      .then(setSystemUsers)
      .catch(() => setSystemUsers([]));
  }, [serverId, server?.useSudo]);

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
      await UploadFilesToServer(serverId, path, asUser);
      await load(path);
    });
  }

  async function handleDownload(entry: sshpool.FileEntry) {
    await withBusy(async () => {
      await DownloadFileFromServer(serverId, entry.path, asUser);
    });
  }

  async function handleDelete() {
    if (selected.size === 0) return;
    if (!confirm(`Hapus ${selected.size} item terpilih? Tindakan ini tidak bisa dibatalkan.`)) return;
    await withBusy(async () => {
      await DeleteFiles(new files.DeleteRequest({ serverId, paths: [...selected], asUser }));
      await load(path);
    });
  }

  async function handleExtract(entry: sshpool.FileEntry) {
    await withBusy(async () => {
      await ExtractArchive(
        new files.ExtractRequest({ serverId, archivePath: entry.path, destPath: path, asUser }),
      );
      await load(path);
    });
  }

  return (
    <div className="files-panel">
      <div className="files-panel__toolbar">
        <button className="btn btn--sm" disabled={path === rootPath || loading} onClick={() => void load(result?.parent ?? rootPath)}>
          {loading ? <span className="spinner" /> : <ArrowUp size={13} />} Naik
        </button>
        <button className="btn btn--sm" disabled={busy} onClick={() => setModal('newFolder')}>
          + Folder
        </button>
        <button className="btn btn--sm" disabled={busy} onClick={() => setModal('newFile')}>
          + File
        </button>
        <button className="btn btn--sm" disabled={busy} onClick={() => void handleUpload()}>
          {busy ? <span className="spinner" /> : <Upload size={13} />} Upload
        </button>
        <button className="btn btn--sm" disabled={busy} onClick={() => setModal('search')}>
          <Search size={13} /> Cari
        </button>
        <button className="btn btn--sm" title="Refresh" disabled={loading} onClick={() => void load(path)}>
          {loading ? <span className="spinner" /> : <RotateCw size={13} />}
        </button>

        {server?.useSudo && (
          <label className="files-panel__asuser">
            <span>Jalankan sebagai</span>
            <select value={asUser} onChange={(e) => setAsUser(e.target.value)}>
              <option value="">{server.username} (SSH)</option>
              {systemUsers
                .filter((u) => u.username !== server.username)
                .map((u) => (
                  <option key={u.username} value={u.username}>
                    {u.username}
                  </option>
                ))}
            </select>
          </label>
        )}

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
            <button className="btn btn--sm" onClick={() => setModal('copy')}>
              Copy
            </button>
            <button className="btn btn--sm" onClick={() => setModal('compress')}>
              Kompres
            </button>
            <button className="btn btn--sm btn--danger" disabled={busy} onClick={() => void handleDelete()}>
              {busy && <span className="spinner" />} Hapus
            </button>
          </div>
        )}
      </div>

      <div className="files-panel__breadcrumb">
        {breadcrumbParts(path, rootPath).map((part, i) => (
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
                  <span className="files-panel__icon">{entry.isDir ? <Folder size={13} /> : <FileText size={13} />}</span>
                  {entry.name}
                </td>
                <td>{formatSize(entry.size, entry.isDir)}</td>
                <td>{formatDate(entry.modTime)}</td>
                <td className="files-panel__row-actions">
                  {!entry.isDir && (
                    <button title="Download" onClick={() => void handleDownload(entry)}>
                      <Download size={14} />
                    </button>
                  )}
                  {!entry.isDir && entry.size <= EDITABLE_SIZE_HINT && (
                    <button title="Edit isi file" onClick={() => setEditTarget(entry)}>
                      <FilePenLine size={14} />
                    </button>
                  )}
                  {!entry.isDir && ARCHIVE_RE.test(entry.name) && (
                    <button title="Ekstrak di sini" disabled={busy} onClick={() => void handleExtract(entry)}>
                      {busy ? <span className="spinner" /> : <Package size={14} />}
                    </button>
                  )}
                  <button title="Ubah permission" onClick={() => setChmodTarget(entry)}>
                    <Lock size={14} />
                  </button>
                  <button title="Rename" onClick={() => setRenameTarget(entry)}>
                    <Pencil size={14} />
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
            await CreateFolder(new files.MkdirRequest({ serverId, path, name, asUser }));
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
            await CreateFile(new files.CreateFileRequest({ serverId, path, name, asUser }));
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
                asUser,
              }),
            );
            await load(path);
          }}
        />
      )}

      {modal === 'copy' && (
        <PromptModal
          title="Copy ke..."
          label="Path tujuan"
          initialValue={path}
          confirmLabel="Copy"
          onClose={() => setModal(null)}
          onConfirm={async (destPath) => {
            await CopyFiles(
              new files.CopyRequest({ serverId, sources: [...selected], destPath, asUser }),
            );
            await load(path);
          }}
        />
      )}

      {modal === 'search' && (
        <SearchModal
          serverId={serverId}
          path={path}
          asUser={asUser}
          onClose={() => setModal(null)}
          onOpenParent={(parentPath) => void load(parentPath)}
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
                asUser,
              }),
            );
            await load(path);
          }}
        />
      )}

      {chmodTarget && (
        <ChmodModal
          path={chmodTarget.path}
          currentMode={chmodTarget.mode}
          onClose={() => setChmodTarget(null)}
          onConfirm={async (mode) => {
            await ChmodFile(new files.ChmodRequest({ serverId, path: chmodTarget.path, mode, asUser }));
            await load(path);
          }}
        />
      )}

      {editTarget && (
        <EditFileModal serverId={serverId} path={editTarget.path} asUser={asUser} onClose={() => setEditTarget(null)} />
      )}
    </div>
  );
}
