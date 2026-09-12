import { useEffect, useState } from 'react';
import { useTabsStore } from '../../store/tabs';
import { ServerFormModal } from './ServerFormModal';
import { BackupModal } from './BackupModal';
import { StatusDot } from './StatusDot';
import type { servers } from '../../../wailsjs/go/models';

// Panel kiri: kartu server (bukan daftar teks polos seperti homepoin) +
// tombol tambah. Klik kartu membuka/memfokuskan tab server itu; tombol edit
// & hapus baru muncul saat kartu di-hover supaya daftar tetap ringkas saat
// mengelola banyak server sekaligus.
export function ServersPage() {
  const { servers: list, tabs, statuses, loadServers, openTab, setActiveTab, deleteServer } =
    useTabsStore();
  const [modal, setModal] = useState<{ mode: 'create' | 'edit'; server?: servers.Server } | null>(
    null,
  );
  const [confirmDelete, setConfirmDelete] = useState<servers.Server | null>(null);
  const [showBackup, setShowBackup] = useState(false);

  useEffect(() => {
    void loadServers();
  }, [loadServers]);

  function openOrFocus(server: servers.Server) {
    const existing = tabs.find((t) => t.serverId === server.id);
    if (existing) {
      setActiveTab(existing.id);
      return;
    }
    void openTab(server.id, server.name);
  }

  return (
    <div className="servers-page">
      <div className="servers-page__header">
        <h1>Servers</h1>
        <button className="btn btn--sm" title="Export/Import data (pindah ke perangkat lain)" onClick={() => setShowBackup(true)}>
          ⇄ Backup
        </button>
        <button className="btn btn--primary btn--sm" onClick={() => setModal({ mode: 'create' })}>
          + Tambah
        </button>
      </div>

      <ul className="server-cards">
        {list.map((server) => {
          const tabCount = tabs.filter((t) => t.serverId === server.id).length;
          return (
            <li
              key={server.id}
              className="server-card"
              style={{ borderLeftColor: server.color }}
              onClick={() => openOrFocus(server)}
            >
              <div className="server-card__main">
                <div className="server-card__name">
                  <StatusDot connection={statuses[server.id]?.connection} />
                  {server.name}
                </div>
                <div className="server-card__host">
                  {server.username}@{server.host}:{server.port}
                </div>
                {server.tags.length > 0 && (
                  <div className="server-card__tags">
                    {server.tags.map((t) => (
                      <span key={t} className="tag-chip tag-chip--readonly">
                        {t}
                      </span>
                    ))}
                  </div>
                )}
              </div>

              <div className="server-card__side">
                {tabCount > 0 && <span className="server-card__tab-count">{tabCount} tab</span>}
                <div className="server-card__actions">
                  <button
                    title="Edit"
                    onClick={(e) => {
                      e.stopPropagation();
                      setModal({ mode: 'edit', server });
                    }}
                  >
                    ✎
                  </button>
                  <button
                    title="Hapus"
                    onClick={(e) => {
                      e.stopPropagation();
                      setConfirmDelete(server);
                    }}
                  >
                    🗑
                  </button>
                </div>
              </div>
            </li>
          );
        })}
        {list.length === 0 && (
          <li className="servers-page__empty">
            Belum ada server. Klik <strong>+ Tambah</strong> untuk mulai.
          </li>
        )}
      </ul>

      {modal && (
        <ServerFormModal mode={modal.mode} initial={modal.server} onClose={() => setModal(null)} />
      )}

      {showBackup && (
        <BackupModal
          onClose={() => setShowBackup(false)}
          onImported={() => void loadServers()}
        />
      )}

      {confirmDelete && (
        <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && setConfirmDelete(null)}>
          <div className="modal-card modal-card--small">
            <div className="modal-card__body">
              <p>
                Hapus server <strong>{confirmDelete.name}</strong>? Semua tab yang menunjuk ke
                server ini akan ikut ditutup.
              </p>
            </div>
            <div className="modal-card__footer">
              <button className="btn btn--ghost" onClick={() => setConfirmDelete(null)}>
                Batal
              </button>
              <button
                className="btn btn--danger"
                onClick={() => {
                  void deleteServer(confirmDelete.id);
                  setConfirmDelete(null);
                }}
              >
                Hapus
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
