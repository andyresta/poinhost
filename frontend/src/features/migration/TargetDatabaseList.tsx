import { Fragment, useCallback, useEffect, useState } from 'react';
import { Database as DatabaseIcon, RefreshCw, ChevronDown, ChevronRight } from 'lucide-react';
import { ListWebsiteDatabases, DbXferListTables } from '../../../wailsjs/go/main/App';
import type { website } from '../../../wailsjs/go/models';
import { useT } from '../../i18n';

const nf = new Intl.NumberFormat();

// Daftar database yang SUDAH ADA di server tujuan, hanya untuk dilihat.
//
// Sisi tujuan sebelumnya cuma punya pemilih server, jadi tidak ada cara
// mengetahui apa yang sudah ada di sana sebelum menekan Pindahkan — dan
// sesudahnya pun tidak ada cara memastikan datanya benar-benar mendarat
// selain mempercayai ringkasan verifikasi. Daftar ini menjawab keduanya:
// dibuka sebelum migrasi untuk melihat apa yang akan tertimpa, dan
// disegarkan sesudahnya untuk melihat hasilnya.
//
// Tidak ada checkbox di sini: tidak ada satu pun keputusan migrasi yang
// diambil dari sisi tujuan — nama database tujuan mengikuti sumber (atau
// diketik terpisah untuk satu database), jadi kehadiran kotak centang di
// sini hanya akan menyiratkan pilihan yang tidak ada.
export function TargetDatabaseList({
  serverId,
  engine,
  reloadToken,
}: {
  serverId: string;
  engine: string;
  // Dinaikkan pemanggil setelah migrasi selesai, supaya daftarnya tidak
  // menampilkan keadaan sebelum transfer.
  reloadToken?: number;
}) {
  const t = useT();
  const [databases, setDatabases] = useState<website.DBDatabaseInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [tablesByDB, setTablesByDB] = useState<Record<string, { tables: string[]; counts: Record<string, number>; note: string }>>({});
  const [tablesLoading, setTablesLoading] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!serverId || !engine) {
      setDatabases([]);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      setDatabases((await ListWebsiteDatabases(serverId, engine)) ?? []);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [serverId, engine]);

  // Ganti server atau engine berarti daftar yang lain sama sekali, jadi
  // tabel yang sudah terbaca ikut dibuang — kalau tidak, angka dari server
  // sebelumnya akan tampil di bawah nama database server baru.
  useEffect(() => {
    void load();
    setExpanded(null);
    setTablesByDB({});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, engine]);

  useEffect(() => {
    if (!reloadToken) return;
    void load();
    setTablesByDB({});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reloadToken]);

  async function toggleExpand(name: string) {
    if (expanded === name) {
      setExpanded(null);
      return;
    }
    setExpanded(name);
    if (tablesByDB[name]) return;
    setTablesLoading(name);
    try {
      const res = await DbXferListTables(serverId, engine, name);
      setTablesByDB((m) => ({
        ...m,
        [name]: { tables: res.tables ?? [], counts: res.counts ?? {}, note: res.countsError ?? '' },
      }));
    } catch (e) {
      setError(String(e));
    } finally {
      setTablesLoading(null);
    }
  }

  return (
    <div className="mig-browser">
      <div className="mig-browser__head">
        <span className="mig-browser__label">{t('migration.db.targetExisting')}</span>
        <button
          className="btn btn--sm"
          title={t('common.refresh')}
          disabled={!serverId || loading}
          onClick={() => void load()}
        >
          <RefreshCw size={13} />
        </button>
      </div>

      <div className="mig-browser__list">
        <div className="mig-browser__scroll">
          {!serverId && <div className="mig-browser__empty">{t('migration.pickServerFirst')}</div>}
          {serverId && error && <div className="mig-browser__error">{error}</div>}
          {serverId && !error && (
            <table className="mig-browser__table">
              <tbody>
                {databases.map((db) => {
                  const open = expanded === db.name;
                  const detail = tablesByDB[db.name];
                  return (
                    <Fragment key={db.name}>
                      <tr>
                        <td className="mig-browser__name">
                          <button className="dbmig__toggle" onClick={() => void toggleExpand(db.name)}>
                            {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                            <DatabaseIcon size={13} />
                            <span className="mig-browser__ellipsis">{db.name}</span>
                          </button>
                        </td>
                      </tr>
                      {open && (
                        <tr>
                          <td className="dbmig__rows-cell">
                            {tablesLoading === db.name && (
                              <div className="mig-browser__empty">{t('common.loading')}</div>
                            )}
                            {detail && detail.tables.length === 0 && (
                              <div className="mig-browser__empty">{t('migration.db.noTable')}</div>
                            )}
                            {detail && detail.tables.length > 0 && (
                              <table className="dbmig__rows">
                                <tbody>
                                  {detail.tables.map((tbl) => (
                                    <tr key={tbl}>
                                      <td>{tbl}</td>
                                      <td className="dbmig__num">
                                        {detail.note ? '—' : nf.format(detail.counts[tbl] ?? 0)}
                                      </td>
                                    </tr>
                                  ))}
                                </tbody>
                              </table>
                            )}
                            {detail && detail.note && (
                              <div className="dbmig__count-note">{t('migration.db.countsUnavailable')}</div>
                            )}
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  );
                })}
                {!loading && databases.length === 0 && (
                  <tr>
                    <td className="mig-browser__empty">{t('migration.db.noDatabase')}</td>
                  </tr>
                )}
              </tbody>
            </table>
          )}
        </div>
        {loading && <div className="mig-browser__loading">{t('common.loading')}</div>}
      </div>
    </div>
  );
}
