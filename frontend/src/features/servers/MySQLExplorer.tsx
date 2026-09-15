import { useEffect, useState } from 'react';
import { Database, RotateCw, Trash2, ChevronLeft, ChevronRight } from 'lucide-react';
import {
  MySQLExploreListDatabases,
  MySQLExploreListTables,
  MySQLExploreListColumns,
  MySQLExploreTableRows,
  MySQLExploreInsertRow,
  MySQLExploreUpdateRow,
  MySQLExploreDeleteRow,
  MySQLExploreExecuteQuery,
  SaveWebsiteDBCredential,
} from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';
import { useConfirm } from '../../components/ConfirmDialog';
import { SqlQueryBox } from './SqlQueryBox';
import { useT } from '../../i18n';

const PAGE_SIZE = 100;
// Halaman hasil kotak query dibuat lebih kecil dari browse tabel: hasil
// query bebas bisa punya banyak kolom lebar (JOIN, SELECT *), dan area
// tampilnya juga lebih pendek.
const QUERY_PAGE_SIZE = 50;

// Explorer MySQL — koneksi driver ASLI (go-sql-driver/mysql) ditunnel lewat
// SSH, BUKAN exec CLI per halaman, supaya paginasi tabel besar tetap cepat
// (lihat mysqlexplore.go). Kalau password yang tersimpan di vault lokal
// ternyata salah/basi (diganti langsung di server), backend menandainya
// dengan prefix "AUTENTIKASI_GAGAL:" — komponen ini mendeteksinya dan
// menawarkan form untuk menyimpan ulang, bukan cuma gagal diam-diam seperti
// di homepoin.
//
// Dulu ini modal terpisah (MySQLExplorerModal) — dipindah jadi konten tab
// "Data" di DatabaseManagerPanel (bukan lagi overlay) supaya jelas beda
// dari tab "Users & Akses": satu urus SIAPA yang boleh akses, satu urus
// BROWSE isinya — dua concern yang sebelumnya campur di satu layar.
export function MySQLExplorer({ serverId, username, host }: { serverId: string; username: string; host: string }) {
  const confirm = useConfirm();
  const t = useT();
  const req: website.MySQLExploreRequest = { serverId, username, host };

  const [databases, setDatabases] = useState<string[] | null>(null);
  const [selectedDb, setSelectedDb] = useState<string | null>(null);
  const [tables, setTables] = useState<website.MySQLTableInfo[]>([]);
  const [selectedTable, setSelectedTable] = useState<string | null>(null);
  const [columns, setColumns] = useState<website.MySQLColumnInfo[]>([]);
  const [rowsResult, setRowsResult] = useState<website.MySQLTableRowsResult | null>(null);
  const [page, setPage] = useState(0);

  const [error, setError] = useState<string | null>(null);
  const [needsPassword, setNeedsPassword] = useState(false);
  const [reenterPassword, setReenterPassword] = useState('');
  const [busy, setBusy] = useState(false);

  const [editingCell, setEditingCell] = useState<{ row: number; col: string } | null>(null);
  const [editingValue, setEditingValue] = useState('');

  const [showInsertForm, setShowInsertForm] = useState(false);
  const [insertValues, setInsertValues] = useState<Record<string, string>>({});

  const [queryText, setQueryText] = useState('');
  const [queryResult, setQueryResult] = useState<website.MySQLQueryResult | null>(null);
  const [queryBusy, setQueryBusy] = useState(false);
  const [queryOffset, setQueryOffset] = useState(0);

  function handleError(e: unknown) {
    const msg = String(e);
    if (msg.includes('AUTENTIKASI_GAGAL')) {
      setNeedsPassword(true);
      setError(t('explore.passwordInvalid'));
    } else {
      setError(msg);
    }
  }

  async function retrySavePassword() {
    if (!reenterPassword.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await SaveWebsiteDBCredential(
        new website.SaveDBCredentialRequest({ serverId, engine: 'mysql', username, host, password: reenterPassword, verify: true }),
      );
      setNeedsPassword(false);
      setReenterPassword('');
      await loadDatabases();
    } catch (e) {
      handleError(e);
    } finally {
      setBusy(false);
    }
  }

  async function loadDatabases() {
    setError(null);
    try {
      setDatabases(await MySQLExploreListDatabases(req));
    } catch (e) {
      handleError(e);
    }
  }

  useEffect(() => {
    setDatabases(null);
    setSelectedDb(null);
    setSelectedTable(null);
    setRowsResult(null);
    setNeedsPassword(false);
    void loadDatabases();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, username, host]);

  async function openDatabase(db: string) {
    if (db === selectedDb) return; // sudah terbuka, tidak perlu ambil ulang daftar tabel
    setSelectedDb(db);
    setSelectedTable(null);
    setColumns([]);
    setRowsResult(null);
    setError(null);
    try {
      setTables(await MySQLExploreListTables(req, db));
    } catch (e) {
      handleError(e);
    }
  }

  // loadRows mengambil SATU halaman baris — dipakai untuk paginasi (skipTotal
  // true: baris tidak berubah jumlahnya, tidak perlu SELECT COUNT(*) ulang
  // yang mahal untuk tabel besar) dan sesudah edit/insert/delete satu baris
  // (skipTotal sesuai apakah jumlah baris ikut berubah). SENGAJA tidak
  // menyentuh kolom sama sekali — struktur tabel tidak berubah hanya karena
  // pindah halaman atau edit satu baris.
  async function loadRows(table: string, targetPage: number, opts: { skipTotal?: boolean } = {}) {
    if (!selectedDb) return;
    setError(null);
    try {
      const res = await MySQLExploreTableRows(
        new website.MySQLTableRowsRequest({
          serverId, username, host, database: selectedDb, table,
          limit: PAGE_SIZE, offset: targetPage * PAGE_SIZE,
          skipTotal: opts.skipTotal ?? false,
        }),
      );
      setPage(targetPage);
      setRowsResult((prev) => (res.total < 0 && prev ? { ...res, total: prev.total } : res));
    } catch (e) {
      handleError(e);
    }
  }

  // openTable dipanggil HANYA saat benar-benar pindah ke tabel lain — inilah
  // satu-satunya titik yang mengambil ulang struktur kolom + total baris
  // (SELECT COUNT(*)), supaya paginasi/edit di dalam tabel yang sama
  // (loadRows) tidak perlu mengulanginya lagi.
  async function openTable(table: string) {
    if (!selectedDb) return;
    if (table === selectedTable) return; // sudah terbuka, tidak perlu ambil ulang struktur/total
    setSelectedTable(table);
    setEditingCell(null);
    setShowInsertForm(false);
    setError(null);
    try {
      setColumns(await MySQLExploreListColumns(req, selectedDb, table));
    } catch (e) {
      handleError(e);
      return;
    }
    await loadRows(table, 0, { skipTotal: false });
  }

  function primaryKeyCols(): string[] {
    const pk = columns.filter((c) => c.key === 'PRI').map((c) => c.name);
    return pk.length > 0 ? pk : columns.map((c) => c.name);
  }

  function rowWhere(row: (string | null)[] | undefined | null): Record<string, string | null> {
    const where: Record<string, string | null> = {};
    if (!rowsResult || !row) return where;
    for (const col of primaryKeyCols()) {
      const idx = rowsResult.columns.indexOf(col);
      if (idx >= 0) where[col] = row[idx] ?? null;
    }
    return where;
  }

  async function commitCellEdit(rowIndex: number) {
    if (!editingCell || !rowsResult || !selectedDb || !selectedTable) return;
    const row = rowsResult.rows[rowIndex];
    const where = rowWhere(row);
    setBusy(true);
    setError(null);
    try {
      await MySQLExploreUpdateRow(
        new website.MySQLRowMutateRequest({
          serverId, username, host, database: selectedDb, table: selectedTable,
          values: { [editingCell.col]: editingValue },
          where,
        }),
      );
      // Edit satu baris tidak mengubah jumlah total baris — skip COUNT(*).
      await loadRows(selectedTable, page, { skipTotal: true });
    } catch (e) {
      handleError(e);
    } finally {
      setBusy(false);
      setEditingCell(null);
    }
  }

  async function handleDeleteRow(rowIndex: number) {
    if (!rowsResult || !selectedDb || !selectedTable) return;
    if (!(await confirm({ title: t('explore.deleteRow'), message: t('explore.deleteRowConfirm'), confirmLabel: t('common.delete'), danger: true }))) return;
    const where = rowWhere(rowsResult.rows[rowIndex]);
    setBusy(true);
    setError(null);
    try {
      await MySQLExploreDeleteRow(
        new website.MySQLRowMutateRequest({ serverId, username, host, database: selectedDb, table: selectedTable, where }),
      );
      // Hapus baris MENGUBAH jumlah total — hitung ulang kali ini.
      await loadRows(selectedTable, page, { skipTotal: false });
    } catch (e) {
      handleError(e);
    } finally {
      setBusy(false);
    }
  }

  async function handleInsertRow() {
    if (!selectedDb || !selectedTable) return;
    setBusy(true);
    setError(null);
    try {
      const values: Record<string, string | null> = {};
      for (const col of columns) {
        const v = insertValues[col.name];
        values[col.name] = v === undefined || v === '' ? (col.nullable ? null : '') : v;
      }
      await MySQLExploreInsertRow(
        new website.MySQLRowMutateRequest({ serverId, username, host, database: selectedDb, table: selectedTable, values }),
      );
      setShowInsertForm(false);
      setInsertValues({});
      // Baris baru MENGUBAH jumlah total — hitung ulang, dan lompat ke
      // halaman pertama supaya baris yang baru dibuat langsung terlihat.
      await loadRows(selectedTable, 0, { skipTotal: false });
    } catch (e) {
      handleError(e);
    } finally {
      setBusy(false);
    }
  }

  // Paginasi hasil query: yang dikirim ke server cuma limit+offset, dan
  // SERVER yang memotong hasilnya (lihat paginateSelectSQL di backend) —
  // bukan menarik seluruh hasil lalu memotong di sini, yang untuk tabel
  // besar berarti menyeret jutaan baris lewat tunnel SSH.
  async function handleRunQuery(offset = 0) {
    if (!selectedDb || !queryText.trim()) return;
    setQueryBusy(true);
    setError(null);
    try {
      const res = await MySQLExploreExecuteQuery(
        new website.MySQLQueryRequest({
          serverId,
          username,
          host,
          database: selectedDb,
          sql: queryText,
          limit: QUERY_PAGE_SIZE,
          offset,
        }),
      );
      setQueryResult(res);
      setQueryOffset(offset);
    } catch (e) {
      handleError(e);
    } finally {
      setQueryBusy(false);
    }
  }

  const totalPages = rowsResult ? Math.max(1, Math.ceil(rowsResult.total / PAGE_SIZE)) : 1;

  if (needsPassword) {
    return (
      <div>
        <p className="overview__error">{error}</p>
        <label className="form-field">
          <span>{t('explore.reenterPassword', { label: `${username}@${host}` })}</span>
          <input type="password" value={reenterPassword} onChange={(e) => setReenterPassword(e.target.value)} />
        </label>
        <button className="btn btn--primary" disabled={busy || !reenterPassword.trim()} onClick={() => void retrySavePassword()}>
          {busy && <span className="spinner" />} {busy ? t('explore.verifying') : t('explore.saveAndRetry')}
        </button>
      </div>
    );
  }

  return (
    <div className="db-explorer">
      <div className="db-explorer__sidebar">
        <h4>{t('explore.databases')}</h4>
        {databases === null && <p className="workspace__placeholder">{t('common.loading')}</p>}
        {databases?.map((db) => (
          <button
            key={db}
            className={`db-explorer__sidebar-item${selectedDb === db ? ' db-explorer__sidebar-item--active' : ''}`}
            onClick={() => void openDatabase(db)}
          >
            <Database size={13} /> {db}
          </button>
        ))}

        {selectedDb && (
          <>
            <h4>{t('explore.tablesIn', { db: selectedDb })}</h4>
            {tables.map((t) => (
              <button
                key={t.name}
                className={`db-explorer__sidebar-item${selectedTable === t.name ? ' db-explorer__sidebar-item--active' : ''}`}
                onClick={() => void openTable(t.name)}
                title={`≈${t.approxRows} baris`}
              >
                {t.name}
              </button>
            ))}
            {tables.length === 0 && <p className="workspace__placeholder">{t('explore.noTables')}</p>}
          </>
        )}
      </div>

      <div className="db-explorer__main">
        {error && <p className="overview__error">{error}</p>}

        {selectedDb && (
          <div>
            <SqlQueryBox
              value={queryText}
              onChange={setQueryText}
              onRun={(off) => void handleRunQuery(off)}
              busy={queryBusy}
              pageSize={QUERY_PAGE_SIZE}
              offset={queryResult?.offset ?? queryOffset}
              rowCount={queryResult?.rows?.length ?? 0}
              paginated={!!queryResult?.paginated}
              hasMore={!!queryResult?.hasMore}
              database={selectedDb}
            />
            {queryResult && !queryResult.isSelect && (
              <p className="chmod-path">{t('explore.rowsAffected', { count: queryResult.rowsAffected })}</p>
            )}
            {queryResult?.isSelect && queryResult.columns && (
              <div className="db-explorer__grid-wrap" style={{ maxHeight: 220, marginTop: 6 }}>
                <table className="db-explorer__grid">
                  <thead>
                    <tr>
                      {queryResult.columns.map((c) => (
                        <th key={c}>{c}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {(queryResult.rows ?? []).map((row, i) => (
                      <tr key={i}>
                        {row.map((v, j) => (
                          <td key={j} className={v === null ? 'db-explorer__cell--null' : ''}>
                            {v === null ? 'NULL' : v}
                          </td>
                        ))}
                      </tr>
                    ))}
                    {(queryResult.rows ?? []).length === 0 && (
                      <tr>
                        <td colSpan={queryResult.columns.length} className="files-panel__empty">
                          {t('explore.noRows')}
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}

        {!selectedTable && selectedDb && <p className="workspace__placeholder">{t('explore.pickTable')}</p>}

        {selectedTable && rowsResult && (
          <>
            <div className="db-explorer__toolbar">
              <strong>{selectedTable}</strong>
              <span style={{ opacity: 0.6 }}>{t('explore.rowCount', { count: rowsResult.total })}</span>
              <button
                title={t('explore.reload')}
                onClick={() => void loadRows(selectedTable, page, { skipTotal: false })}
              >
                <RotateCw size={14} />
              </button>
              <button className="btn btn--sm btn--primary" style={{ marginLeft: 'auto' }} onClick={() => setShowInsertForm((v) => !v)}>
                {t('explore.addRow')}
              </button>
            </div>

            {showInsertForm && (
              <div className="db-explorer__insert-form">
                {columns.map((c) => (
                  <label key={c.name}>
                    <span>
                      {c.name}
                      {c.key === 'PRI' ? ' (PK)' : ''}
                    </span>
                    <input
                      placeholder={c.nullable ? 'NULL' : ''}
                      value={insertValues[c.name] ?? ''}
                      onChange={(e) => setInsertValues((v) => ({ ...v, [c.name]: e.target.value }))}
                    />
                  </label>
                ))}
                <button className="btn btn--sm btn--primary" disabled={busy} onClick={() => void handleInsertRow()}>
                  {busy && <span className="spinner" />} Simpan
                </button>
              </div>
            )}

            <div className="db-explorer__grid-wrap">
              <table className="db-explorer__grid">
                <thead>
                  <tr>
                    {rowsResult.columns.map((c) => (
                      <th key={c}>{c}</th>
                    ))}
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {rowsResult.rows.map((row, ri) => (
                    <tr key={ri}>
                      {row.map((val, ci) => {
                        const col = rowsResult.columns[ci];
                        const isEditing = editingCell?.row === ri && editingCell?.col === col;
                        return (
                          <td
                            key={ci}
                            className={val === null && !isEditing ? 'db-explorer__cell--null' : ''}
                            onDoubleClick={() => {
                              setEditingCell({ row: ri, col });
                              setEditingValue(val ?? '');
                            }}
                          >
                            {isEditing ? (
                              <input
                                autoFocus
                                className="db-explorer__cell-input"
                                value={editingValue}
                                onChange={(e) => setEditingValue(e.target.value)}
                                onBlur={() => void commitCellEdit(ri)}
                                onKeyDown={(e) => {
                                  if (e.key === 'Enter') void commitCellEdit(ri);
                                  if (e.key === 'Escape') setEditingCell(null);
                                }}
                              />
                            ) : val === null ? (
                              'NULL'
                            ) : (
                              val
                            )}
                          </td>
                        );
                      })}
                      <td>
                        <button title={t('explore.deleteRow')} disabled={busy} onClick={() => void handleDeleteRow(ri)}>
                          <Trash2 size={14} />
                        </button>
                      </td>
                    </tr>
                  ))}
                  {rowsResult.rows.length === 0 && (
                    <tr>
                      <td colSpan={rowsResult.columns.length + 1} className="files-panel__empty">
                        {t('explore.emptyTable')}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            <div className="db-explorer__pagination">
              <button className="btn btn--sm" disabled={page <= 0} onClick={() => void loadRows(selectedTable, page - 1, { skipTotal: true })}>
                <ChevronLeft size={13} /> {t('explore.prev')}
              </button>
              <span>
                {t('explore.page', { page: page + 1, total: totalPages })}
              </span>
              <button
                className="btn btn--sm"
                disabled={page + 1 >= totalPages}
                onClick={() => void loadRows(selectedTable, page + 1, { skipTotal: true })}
              >
                {t('explore.next')} <ChevronRight size={13} />
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
