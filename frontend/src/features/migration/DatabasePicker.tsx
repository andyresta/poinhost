import { Fragment, useCallback, useEffect, useState } from 'react';
import { Database as DatabaseIcon, RefreshCw, ChevronDown, ChevronRight } from 'lucide-react';
import { ListWebsiteDatabases, DbXferListTables } from '../../../wailsjs/go/main/App';
import type { website, servers } from '../../../wailsjs/go/models';
import { useT } from '../../i18n';

// Satu database yang dipilih user, plus subset tabelnya (kalau tidak
// "semua tabel"). Disimpan sebagai Map di komponen induk, keyed by nama
// database — bentuk ini yang paling langsung diterjemahkan ke
// dbxfer.ItemSelection saat submit.
export interface DBSelection {
	allTables: boolean;
	tables: Set<string>;
}

// Pemilih database + engine satu server — padanan RemoteBrowser/
// ContainerPicker tapi untuk migrasi database: server DAN engine
// (MySQL/PostgreSQL) dipilih dulu, lalu database dicentang satu-satu
// (multi-select, beda dari ContainerPicker migrasi Docker yang cuma satu
// container per job — migrasi database memang mendukung banyak database
// sekaligus, lihat ARCHITECTURE.md).
export function DatabasePicker({
  serverList,
  serverId,
  onServerChange,
  engine,
  onEngineChange,
  selected,
  onSelectedChange,
  disabled,
}: {
  serverList: servers.Server[];
  serverId: string;
  onServerChange: (id: string) => void;
  engine: string;
  onEngineChange: (e: string) => void;
  selected: Map<string, DBSelection>;
  onSelectedChange: (next: Map<string, DBSelection>) => void;
  disabled?: boolean;
}) {
  const t = useT();
  const [databases, setDatabases] = useState<website.DBDatabaseInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [tablesByDB, setTablesByDB] = useState<Record<string, string[]>>({});
  const [tablesLoading, setTablesLoading] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!serverId || !engine) {
      setDatabases([]);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const res = await ListWebsiteDatabases(serverId, engine);
      setDatabases(res ?? []);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [serverId, engine]);

  useEffect(() => {
    void load();
    onSelectedChange(new Map());
    setExpanded(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, engine]);

  function toggleDatabase(name: string) {
    const next = new Map(selected);
    if (next.has(name)) {
      next.delete(name);
    } else {
      next.set(name, { allTables: true, tables: new Set() });
    }
    onSelectedChange(next);
  }

  async function toggleExpand(name: string) {
    if (expanded === name) {
      setExpanded(null);
      return;
    }
    setExpanded(name);
    if (!tablesByDB[name]) {
      setTablesLoading(name);
      try {
        const res = await DbXferListTables(serverId, engine, name);
        setTablesByDB((m) => ({ ...m, [name]: res.tables ?? [] }));
      } catch (e) {
        setError(String(e));
      } finally {
        setTablesLoading(null);
      }
    }
  }

  function toggleTable(dbName: string, table: string) {
    const sel = selected.get(dbName);
    if (!sel) return;
    const next = new Map(selected);
    if (sel.allTables) {
      // Dari "semua tabel" ke seleksi eksplisit: mulai dari semua tabel
      // TERCENTANG, lalu lepas yang baru diklik — jadi hasilnya masih
      // "semua kecuali satu", sesuai ekspektasi klik pertama pada mode ini.
      const all = new Set(tablesByDB[dbName] ?? []);
      all.delete(table);
      next.set(dbName, { allTables: false, tables: all });
    } else {
      const tables = new Set(sel.tables);
      if (tables.has(table)) tables.delete(table);
      else tables.add(table);
      next.set(dbName, { allTables: false, tables });
    }
    onSelectedChange(next);
  }

  function resetToAllTables(dbName: string) {
    const next = new Map(selected);
    next.set(dbName, { allTables: true, tables: new Set() });
    onSelectedChange(next);
  }

  return (
    <div className="mig-browser">
      <div className="mig-browser__head">
        <select
          className="mig-browser__server"
          value={serverId}
          disabled={disabled}
          onChange={(e) => onServerChange(e.target.value)}
        >
          <option value="">{t('migration.pickServer')}</option>
          {serverList.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </select>
        <select
          className="mig-browser__server mig-db-picker__engine"
          value={engine}
          disabled={disabled}
          onChange={(e) => onEngineChange(e.target.value)}
        >
          <option value="mysql">MySQL</option>
          <option value="postgresql">PostgreSQL</option>
        </select>
        <button
          className="btn btn--sm"
          title={t('common.refresh')}
          disabled={!serverId || loading || disabled}
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
                {databases.map((d) => {
                  const sel = selected.get(d.name);
                  const isExpanded = expanded === d.name;
                  const tables = tablesByDB[d.name] ?? [];
                  return (
                    <Fragment key={d.name}>
                      <tr>
                        <td className="mig-browser__col-check">
                          <input
                            type="checkbox"
                            checked={!!sel}
                            disabled={disabled}
                            onChange={() => toggleDatabase(d.name)}
                          />
                        </td>
                        <td className="mig-browser__name" onClick={() => sel && void toggleExpand(d.name)}>
                          <span className="mig-browser__icon">
                            <DatabaseIcon size={13} />
                          </span>
                          <span className="mig-browser__ellipsis">{d.name}</span>
                        </td>
                        <td className="mig-browser__size">
                          {sel && (
                            <button
                              className="btn btn--sm mig-db-picker__tablesbtn"
                              disabled={disabled}
                              onClick={() => void toggleExpand(d.name)}
                            >
                              {isExpanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                              {t('migration.db.tables')}
                            </button>
                          )}
                        </td>
                      </tr>
                      {sel && isExpanded && (
                        <tr>
                          <td colSpan={3} className="mig-db-picker__tablepanel">
                            {tablesLoading === d.name && <span>{t('common.loading')}</span>}
                            {tablesLoading !== d.name && (
                              <>
                                <label className="mig-panel__opt">
                                  <input
                                    type="checkbox"
                                    checked={sel.allTables}
                                    disabled={disabled}
                                    onChange={() => (sel.allTables ? undefined : resetToAllTables(d.name))}
                                  />
                                  {t('migration.db.allTables')}
                                </label>
                                <div className="mig-db-picker__tablelist">
                                  {tables.map((tbl) => (
                                    <label key={tbl} className="mig-panel__opt">
                                      <input
                                        type="checkbox"
                                        checked={sel.allTables || sel.tables.has(tbl)}
                                        disabled={disabled}
                                        onChange={() => toggleTable(d.name, tbl)}
                                      />
                                      {tbl}
                                    </label>
                                  ))}
                                  {tables.length === 0 && <span>{t('migration.db.noTables')}</span>}
                                </div>
                              </>
                            )}
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  );
                })}
                {databases.length === 0 && (
                  <tr>
                    <td colSpan={3} className="mig-browser__empty">
                      {t('migration.emptyFolder')}
                    </td>
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
