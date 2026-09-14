import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTabsStore } from '../../store/tabs';
import { useSidebarStore } from '../../store/sidebar';
import { ServerFormModal } from './ServerFormModal';
import { BackupModal } from './BackupModal';
import { StatusDot } from './StatusDot';
import { ThemeToggle } from './ThemeToggle';
import { ArrowLeftRight, Pencil, Plus, Trash2, PanelLeftClose, PanelLeftOpen } from 'lucide-react';
import type { servers } from '../../../wailsjs/go/models';

// Panel kiri: kartu server (bukan daftar teks polos seperti homepoin) +
// tombol tambah. Klik kartu membuka/memfokuskan tab server itu; tombol edit
// & hapus baru muncul saat kartu di-hover supaya daftar tetap ringkas saat
// mengelola banyak server sekaligus.
export function ServersPage() {
  const { servers: list, tabs, statuses, loadServers, openTab, setActiveTab, deleteServer } =
    useTabsStore();
  const collapsed = useSidebarStore((s) => s.collapsed);
  const toggleCollapsed = useSidebarStore((s) => s.toggle);
  const [modal, setModal] = useState<{ mode: 'create' | 'edit'; server?: servers.Server } | null>(
    null,
  );
  const [confirmDelete, setConfirmDelete] = useState<servers.Server | null>(null);
  const [showBackup, setShowBackup] = useState(false);

  // Popover kartu server saat panel diciutkan: dirender lewat portal ke
  // <body> (posisi `fixed`, dihitung dari getBoundingClientRect ikon) —
  // bukan absolute di dalam sidebar, supaya tidak terpotong oleh
  // overflow-y:auto milik .app__sidebar (overflow-x tidak bisa "visible"
  // sendirian selama overflow-y bukan "visible" — browser memaksa
  // keduanya jadi "auto", lihat catatan di App.css).
  const [hoverId, setHoverId] = useState<string | null>(null);
  const [hoverPos, setHoverPos] = useState({ top: 0, left: 0 });
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    void loadServers();
  }, [loadServers]);

  function clearCloseTimer() {
    if (closeTimer.current) {
      clearTimeout(closeTimer.current);
      closeTimer.current = null;
    }
  }

  function scheduleClosePopover() {
    clearCloseTimer();
    closeTimer.current = setTimeout(() => setHoverId(null), 150);
  }

  function openPopover(serverId: string, target: HTMLElement) {
    clearCloseTimer();
    const rect = target.getBoundingClientRect();
    setHoverPos({ top: rect.top, left: rect.right + 8 });
    setHoverId(serverId);
  }

  function openOrFocus(server: servers.Server) {
    const existing = tabs.find((t) => t.serverId === server.id);
    if (existing) {
      setActiveTab(existing.id);
      return;
    }
    void openTab(server.id, server.name);
  }

  function renderCardBody(server: servers.Server, tabCount: number) {
    return (
      <>
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
              <Pencil size={13} />
            </button>
            <button
              title="Hapus"
              onClick={(e) => {
                e.stopPropagation();
                setConfirmDelete(server);
              }}
            >
              <Trash2 size={13} />
            </button>
          </div>
        </div>
      </>
    );
  }

  const hoveredServer = collapsed && hoverId ? list.find((s) => s.id === hoverId) : undefined;

  return (
    <div className={`servers-page${collapsed ? ' servers-page--collapsed' : ''}`}>
      <div className="servers-page__header">
        <div className="servers-page__header-left">
          <button
            className="btn btn--sm servers-page__collapse-toggle"
            title={collapsed ? 'Perluas panel' : 'Ciutkan panel'}
            aria-label={collapsed ? 'Perluas panel' : 'Ciutkan panel'}
            onClick={toggleCollapsed}
          >
            {collapsed ? <PanelLeftOpen size={14} /> : <PanelLeftClose size={14} />}
          </button>
          {!collapsed && <h1>Servers</h1>}
        </div>
        <div className="servers-page__header-actions">
          <ThemeToggle />
          <button className="btn btn--sm" title="Export/Import data (pindah ke perangkat lain)" onClick={() => setShowBackup(true)}>
            <ArrowLeftRight size={13} /> {!collapsed && 'Backup'}
          </button>
          <button className="btn btn--primary btn--sm" title="Tambah server" onClick={() => setModal({ mode: 'create' })}>
            <Plus size={13} /> {!collapsed && 'Tambah'}
          </button>
        </div>
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
              onMouseEnter={(e) => collapsed && openPopover(server.id, e.currentTarget)}
              onMouseLeave={() => collapsed && scheduleClosePopover()}
            >
              <div className="server-card__icon" style={{ background: server.color }} title={server.name}>
                <StatusDot connection={statuses[server.id]?.connection} />
              </div>

              {!collapsed && (
                <div className="server-card__body">{renderCardBody(server, tabCount)}</div>
              )}
            </li>
          );
        })}
        {list.length === 0 && !collapsed && (
          <li className="servers-page__empty">
            Belum ada server. Klik <strong>+ Tambah</strong> untuk mulai.
          </li>
        )}
      </ul>

      {hoveredServer &&
        createPortal(
          <div
            className="server-card-popover"
            style={{ top: hoverPos.top, left: hoverPos.left }}
            onMouseEnter={clearCloseTimer}
            onMouseLeave={scheduleClosePopover}
          >
            {renderCardBody(hoveredServer, tabs.filter((t) => t.serverId === hoveredServer.id).length)}
          </div>,
          document.body,
        )}

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
