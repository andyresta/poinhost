import { useState } from 'react';
import { ExportBackup, ImportBackup } from '../../../wailsjs/go/main/App';
import { backup } from '../../../wailsjs/go/models';

// Mekanisme pindah data antar perangkat TANPA akun/server/layanan pihak
// ketiga (lihat internal/core/backup) — satu file terenkripsi passphrase,
// user sendiri yang membawanya lewat kanal apa pun yang mereka percaya.
// Passphrase TIDAK PERNAH disimpan poinhost — kalau lupa, arsipnya tidak
// bisa dibuka lagi sama sekali, jadi peringatan ini ditampilkan tegas.
export function BackupModal({ onClose, onImported }: { onClose: () => void; onImported: () => void }) {
  const [tab, setTab] = useState<'export' | 'import'>('export');

  const [exportPass, setExportPass] = useState('');
  const [exportPassConfirm, setExportPassConfirm] = useState('');
  const [includeKeyFiles, setIncludeKeyFiles] = useState(false);
  const [exportBusy, setExportBusy] = useState(false);
  const [exportResult, setExportResult] = useState<string | null>(null);
  const [exportError, setExportError] = useState<string | null>(null);

  const [importPass, setImportPass] = useState('');
  const [importBusy, setImportBusy] = useState(false);
  const [importSummary, setImportSummary] = useState<backup.ImportSummary | null>(null);
  const [importError, setImportError] = useState<string | null>(null);

  async function handleExport() {
    if (exportPass.length < 8) {
      setExportError('Passphrase minimal 8 karakter.');
      return;
    }
    if (exportPass !== exportPassConfirm) {
      setExportError('Konfirmasi passphrase tidak cocok.');
      return;
    }
    setExportBusy(true);
    setExportError(null);
    setExportResult(null);
    try {
      const path = await ExportBackup(exportPass, includeKeyFiles);
      if (path) setExportResult(path);
    } catch (e) {
      setExportError(String(e));
    } finally {
      setExportBusy(false);
    }
  }

  async function handleImport() {
    if (!importPass) {
      setImportError('Masukkan passphrase arsip terlebih dulu.');
      return;
    }
    setImportBusy(true);
    setImportError(null);
    setImportSummary(null);
    try {
      const summary = await ImportBackup(importPass);
      if (summary) {
        setImportSummary(summary);
        onImported();
      }
    } catch (e) {
      setImportError(String(e));
    } finally {
      setImportBusy(false);
    }
  }

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card">
        <div className="modal-card__header">
          <h2>Backup & Pindah Perangkat</h2>
          <button className="modal-card__close" onClick={onClose}>
            ×
          </button>
        </div>

        <div className="segmented" style={{ margin: '0 16px' }}>
          <button className={`segmented__item${tab === 'export' ? ' segmented__item--active' : ''}`} onClick={() => setTab('export')}>
            Export
          </button>
          <button className={`segmented__item${tab === 'import' ? ' segmented__item--active' : ''}`} onClick={() => setTab('import')}>
            Import
          </button>
        </div>

        <div className="modal-card__body">
          {tab === 'export' && (
            <>
              <p className="chmod-path">
                Membuat satu file berisi semua server, kredensial database tersimpan, dan tautan
                domain-database — dienkripsi dengan passphrase di bawah. Bawa file ini sendiri
                (USB/Drive pribadi/dst) ke perangkat lain, lalu Import di sana dengan passphrase
                yang sama.
              </p>
              <p className="overview__error" style={{ opacity: 0.85 }}>
                Passphrase TIDAK disimpan poinhost — kalau lupa, arsipnya tidak bisa dibuka lagi.
              </p>
              <label className="form-field">
                <span>Passphrase</span>
                <input type="password" value={exportPass} onChange={(e) => setExportPass(e.target.value)} />
              </label>
              <label className="form-field">
                <span>Konfirmasi passphrase</span>
                <input type="password" value={exportPassConfirm} onChange={(e) => setExportPassConfirm(e.target.value)} />
              </label>
              <label className="form-check">
                <input type="checkbox" checked={includeKeyFiles} onChange={(e) => setIncludeKeyFiles(e.target.checked)} />
                <span>Sertakan isi private key SSH (server dengan auth kunci) di dalam arsip</span>
              </label>

              {exportError && <p className="overview__error">{exportError}</p>}
              {exportResult && <p className="chmod-path">Tersimpan di: {exportResult}</p>}

              <button className="btn btn--primary" disabled={exportBusy} onClick={() => void handleExport()}>
                {exportBusy ? 'Membuat arsip…' : 'Pilih Lokasi & Simpan'}
              </button>
            </>
          )}

          {tab === 'import' && (
            <>
              <p className="chmod-path">
                Pilih file arsip yang dibuat lewat Export (di perangkat mana pun), lalu masukkan
                passphrase yang sama saat itu dibuat. Server yang ID-nya sudah ada di perangkat ini
                akan diperbarui, yang belum ada akan ditambahkan.
              </p>
              <label className="form-field">
                <span>Passphrase arsip</span>
                <input type="password" value={importPass} onChange={(e) => setImportPass(e.target.value)} />
              </label>

              {importError && <p className="overview__error">{importError}</p>}
              {importSummary && (
                <p className="chmod-path">
                  Berhasil: {importSummary.serversImported} server, {importSummary.dbCredentialsImported} kredensial
                  database, {importSummary.domainLinksImported} tautan domain-database.
                </p>
              )}

              <button className="btn btn--primary" disabled={importBusy} onClick={() => void handleImport()}>
                {importBusy ? 'Mengimpor…' : 'Pilih File & Import'}
              </button>
            </>
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
