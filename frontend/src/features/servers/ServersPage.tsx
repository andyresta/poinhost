import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTabsStore } from '../../store/tabs';
import { useSidebarStore } from '../../store/sidebar';
import { ServerFormModal } from './ServerFormModal';
import { BackupModal } from './BackupModal';
import { StatusDot } from './StatusDot';
import { ThemeToggle } from './ThemeToggle';
import { LangToggle } from './LangToggle';
import { useT } from '../../i18n';
import { ArrowLeftRight, Archive, Pencil, Plus, Trash2 } from 'lucide-react';
import type { servers } from '../../../wailsjs/go/models';

// Panel kiri: kartu server (bukan daftar teks polos seperti homepoin) +
// tombol tambah. Klik kartu membuka/memfokuskan tab server itu; tombol edit
// & hapus baru muncul saat kartu di-hover supaya daftar tetap ringkas saat
// mengelola banyak server sekaligus.
export function ServersPage() {
  const { servers: list, tabs, statuses, loadServers, openTab, openMigrationTab, setActiveTab, deleteServer } =
    useTabsStore();
  const t = useT();
  const collapsed = useSidebarStore((s) => s.collapsed);
  const [modal, setModal] = useState<{ mode: 'create' | 'edit'; server?: servers.Server } | null>(
    null,
  );
  const [confirmDelete, setConfirmDelete] = useState<servers.Server | null>(null);
  const [showBackup, setShowBackup] = useState(false);
  const migrationTabTitle = t('migration.title');

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

  // Satu tab migrasi sudah cukup: semua pemilihan server ada di dalam
  // panelnya, jadi tab kedua hanya akan menduplikasi formulir yang sama.
  function openOrFocusMigration() {
    const existing = tabs.find((t) => t.kind === 'migration');
    if (existing) {
      setActiveTab(existing.id);
      return;
    }
    void openMigrationTab(migrationTabTitle);
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
          {tabCount > 0 && <span className="server-card__tab-count">{t('sidebar.tabCount', { count: tabCount })}</span>}
          <div className="server-card__actions">
            <button
              title={t('sidebar.edit')}
              onClick={(e) => {
                e.stopPropagation();
                setModal({ mode: 'edit', server });
              }}
            >
              <Pencil size={13} />
            </button>
            <button
              title={t('sidebar.delete')}
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
        <div className="servers-page__header-left">{!collapsed && <h1>PoinHost</h1>}</div>
        <div className="servers-page__header-toggles">
          <button
            className="btn btn--sm"
            title={t('sidebar.backupTitle')}
            aria-label={t('sidebar.backup')}
            onClick={() => setShowBackup(true)}
          >
            <Archive size={14} />
          </button>
          <LangToggle />
          <ThemeToggle />
        </div>
      </div>

      {/* Aksi panel (tambah server / backup) sengaja SEBARIS SENDIRI di
          bawah judul, bukan berdesakan dengan judul + toggle tema: di lebar
          sidebar 260px ketiganya dalam satu baris bikin label tombolnya
          terpotong. */}
      <div className="servers-page__actions">
        <button className="btn btn--primary btn--sm" title={t('sidebar.addServer')} onClick={() => setModal({ mode: 'create' })}>
          <Plus size={13} /> {!collapsed && t('common.add')}
        </button>
        <button className="btn btn--sm" title={t('sidebar.migrationTitle')} onClick={openOrFocusMigration}>
          <ArrowLeftRight size={13} /> {!collapsed && t('sidebar.migration')}
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
          <li className="servers-page__empty">{t('sidebar.empty')}</li>
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
                {t('sidebar.deleteConfirm', { name: confirmDelete.name })}
              </p>
            </div>
            <div className="modal-card__footer">
              <button className="btn btn--ghost" onClick={() => setConfirmDelete(null)}>
                {t('common.cancel')}
              </button>
              <button
                className="btn btn--danger"
                onClick={() => {
                  void deleteServer(confirmDelete.id);
                  setConfirmDelete(null);
                }}
              >
                {t('common.delete')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
