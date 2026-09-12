import { useEffect, useState } from 'react';
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

const PAGE_SIZE = 100;

// Explorer MySQL — koneksi driver ASLI (go-sql-driver/mysql) ditunnel lewat
// SSH, BUKAN exec CLI per halaman, supaya paginasi tabel besar tetap cepat
// (lihat mysqlexplore.go). Kalau password yang tersimpan di vault lokal
// ternyata salah/basi (diganti langsung di server), backend menandainya
// dengan prefix "AUTENTIKASI_GAGAL:" — modal ini mendeteksinya dan
// menawarkan form untuk menyimpan ulang, bukan cuma gagal diam-diam seperti
// di homepoin.
export function MySQLExplorerModal({
  serverId,
  username,
  host,
  onClose,
}: {
  serverId: string;
  username: string;
  host: string;
  onClose: () => void;
}) {
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

  function handleError(e: unknown) {
    const msg = String(e);
    if (msg.includes('AUTENTIKASI_GAGAL')) {
      setNeedsPassword(true);
      setError('Password tersimpan sudah tidak valid (mungkin diganti langsung di server).');
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
    if (!confirm('Hapus baris ini?')) return;
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

  async function handleRunQuery() {
    if (!selectedDb || !queryText.trim()) return;
    setQueryBusy(true);
    setError(null);
    try {
      setQueryResult(await MySQLExploreExecuteQuery(new website.MySQLQueryRequest({ serverId, username, host, database: selectedDb, sql: queryText })));
    } catch (e) {
      handleError(e);
    } finally {
      setQueryBusy(false);
    }
  }

  const totalPages = rowsResult ? Math.max(1, Math.ceil(rowsResult.total / PAGE_SIZE)) : 1;

  return (
    <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card modal-card--explorer">
        <div className="modal-card__header">
          <h2>
            Explore MySQL — {username}@{host}
          </h2>
          <button className="modal-card__close" onClick={onClose}>
            ×
          </button>
        </div>

        {needsPassword ? (
          <div className="modal-card__body">
            <p className="overview__error">{error}</p>
            <label className="form-field">
              <span>Masukkan ulang password untuk {username}@{host}</span>
              <input type="password" value={reenterPassword} onChange={(e) => setReenterPassword(e.target.value)} />
            </label>
            <button className="btn btn--primary" disabled={busy || !reenterPassword.trim()} onClick={() => void retrySavePassword()}>
              {busy ? 'Memverifikasi…' : 'Simpan & Coba Lagi'}
            </button>
          </div>
        ) : (
          <div className="db-explorer">
            <div className="db-explorer__sidebar">
              <h4>Database</h4>
              {databases === null && <p className="workspace__placeholder">Memuat…</p>}
              {databases?.map((db) => (
                <button
                  key={db}
                  className={`db-explorer__sidebar-item${selectedDb === db ? ' db-explorer__sidebar-item--active' : ''}`}
                  onClick={() => void openDatabase(db)}
                >
                  🗄 {db}
                </button>
              ))}

              {selectedDb && (
                <>
                  <h4>Tabel di {selectedDb}</h4>
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
                  {tables.length === 0 && <p className="workspace__placeholder">Tidak ada tabel.</p>}
                </>
              )}
            </div>

            <div className="db-explorer__main">
              {error && <p className="overview__error">{error}</p>}

              {selectedDb && (
                <div>
                  <textarea
                    className="db-explorer__query-box"
                    placeholder={`Jalankan query bebas di database "${selectedDb}" (satu statement)…`}
                    value={queryText}
                    onChange={(e) => setQueryText(e.target.value)}
                  />
                  <div className="db-explorer__toolbar">
                    <button className="btn btn--sm btn--primary" disabled={queryBusy || !queryText.trim()} onClick={() => void handleRunQuery()}>
                      {queryBusy ? 'Menjalankan…' : 'Jalankan Query'}
                    </button>
                    {queryResult && !queryResult.isSelect && <span>{queryResult.rowsAffected} baris terpengaruh.</span>}
                  </div>
                  {queryResult?.isSelect && queryResult.columns && (
                    <div className="db-explorer__grid-wrap" style={{ maxHeight: 180, marginTop: 6 }}>
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
                        </tbody>
                      </table>
                    </div>
                  )}
                </div>
              )}

              {!selectedTable && selectedDb && <p className="workspace__placeholder">Pilih tabel di panel kiri untuk browse isinya.</p>}

              {selectedTable && rowsResult && (
                <>
                  <div className="db-explorer__toolbar">
                    <strong>{selectedTable}</strong>
                    <span style={{ opacity: 0.6 }}>{rowsResult.total} baris</span>
                    <button
                      title="Muat ulang halaman ini + hitung ulang total (data mungkin berubah dari tempat lain)"
                      onClick={() => void loadRows(selectedTable, page, { skipTotal: false })}
                    >
                      ⟳
                    </button>
                    <button className="btn btn--sm btn--primary" style={{ marginLeft: 'auto' }} onClick={() => setShowInsertForm((v) => !v)}>
                      + Baris
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
                        Simpan
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
                              <button title="Hapus baris" disabled={busy} onClick={() => void handleDeleteRow(ri)}>
                                🗑
                              </button>
                            </td>
                          </tr>
                        ))}
                        {rowsResult.rows.length === 0 && (
                          <tr>
                            <td colSpan={rowsResult.columns.length + 1} className="files-panel__empty">
                              Tabel kosong.
                            </td>
                          </tr>
                        )}
                      </tbody>
                    </table>
                  </div>

                  <div className="db-explorer__pagination">
                    <button className="btn btn--sm" disabled={page <= 0} onClick={() => void loadRows(selectedTable, page - 1, { skipTotal: true })}>
                      ← Sebelumnya
                    </button>
                    <span>
                      Halaman {page + 1} / {totalPages}
                    </span>
                    <button
                      className="btn btn--sm"
                      disabled={page + 1 >= totalPages}
                      onClick={() => void loadRows(selectedTable, page + 1, { skipTotal: true })}
                    >
                      Berikutnya →
                    </button>
                  </div>
                </>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
