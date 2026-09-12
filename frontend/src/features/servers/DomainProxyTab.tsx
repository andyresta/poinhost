import { useEffect, useState } from 'react';
import {
  GetWebsiteProxyStatus,
  SetWebsiteProxyDomain,
  DisableWebsiteProxyDomain,
  SetWebsiteProxyRule,
  DeleteWebsiteProxyRule,
} from '../../../wailsjs/go/main/App';
import { website } from '../../../wailsjs/go/models';

// Tab Proxy — mengedit field ProxyTarget/ProxyRules yang SAMA sudah ada di
// vhost domain (dipakai juga oleh PHP/SSL) — bukan mekanisme baru, cuma
// UI+API untuk mengubahnya. Reverse-proxy port-forward server-wide (menu
// terpisah di homepoin, tidak terikat satu domain) tidak ada di sini.
export function DomainProxyTab({ serverId, domain }: { serverId: string; domain: string }) {
  const [status, setStatus] = useState<website.ProxyStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [target, setTarget] = useState('');
  const [ws, setWs] = useState(false);
  const [rulePath, setRulePath] = useState('');
  const [ruleTarget, setRuleTarget] = useState('');
  const [ruleWs, setRuleWs] = useState(false);

  async function load() {
    try {
      const st = await GetWebsiteProxyStatus(serverId, domain);
      setStatus(st);
      setTarget(st.proxyTarget ?? '');
      setWs(false);
    } catch (e) {
      setError(String(e));
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, domain]);

  async function handleEnable() {
    if (!target.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await SetWebsiteProxyDomain(new website.ProxySetDomainRequest({ serverId, domain, target: target.trim(), webSocket: ws }));
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleDisable() {
    setBusy(true);
    setError(null);
    try {
      await DisableWebsiteProxyDomain(serverId, domain);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleAddRule() {
    if (!rulePath.trim() || !ruleTarget.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await SetWebsiteProxyRule(
        new website.ProxyRuleRequest({ serverId, domain, path: rulePath.trim(), target: ruleTarget.trim(), webSocket: ruleWs }),
      );
      setRulePath('');
      setRuleTarget('');
      setRuleWs(false);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  async function handleDeleteRule(path: string) {
    setBusy(true);
    setError(null);
    try {
      await DeleteWebsiteProxyRule(serverId, domain, path);
      await load();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  if (!status) return <p className="workspace__placeholder">Memuat…</p>;

  return (
    <div>
      {error && <p className="overview__error">{error}</p>}

      <p>
        Mode saat ini:{' '}
        <span className="docker-badge docker-badge--running">
          {{ static: 'Static', php: 'PHP', proxy: 'Proxy (seluruh domain)', mixed: 'Campuran (rule per-path)' }[status.mode]}
        </span>
      </p>

      <section style={{ marginBottom: 16 }}>
        <h3 style={{ fontSize: 12, textTransform: 'uppercase', opacity: 0.6, margin: '0 0 8px' }}>Proxy seluruh domain</h3>
        {status.proxyTarget ? (
          <>
            <p className="chmod-path">Semua traffic domain ini diteruskan ke: {status.proxyTarget}</p>
            <button className="btn btn--sm btn--danger" disabled={busy} onClick={() => void handleDisable()}>
              Nonaktifkan (kembali ke {status.phpEnabled ? `PHP ${status.phpVersion}` : 'static'})
            </button>
          </>
        ) : (
          <div className="docker-recreate__row">
            <input placeholder="http://127.0.0.1:3000" value={target} onChange={(e) => setTarget(e.target.value)} />
            <label className="docker-recreate__ro">
              <input type="checkbox" checked={ws} onChange={(e) => setWs(e.target.checked)} />
              websocket
            </label>
            <button className="btn btn--sm btn--primary" disabled={busy || !target.trim()} onClick={() => void handleEnable()}>
              Aktifkan
            </button>
          </div>
        )}
      </section>

      <section>
        <h3 style={{ fontSize: 12, textTransform: 'uppercase', opacity: 0.6, margin: '0 0 8px' }}>Aturan per-path</h3>
        <table className="files-panel__table">
          <thead>
            <tr>
              <th>Path</th>
              <th>Target</th>
              <th>WS</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {(status.proxyRules ?? []).map((r) => (
              <tr key={r.path}>
                <td>{r.path}</td>
                <td>{r.target}</td>
                <td>{r.webSocket ? '✓' : '—'}</td>
                <td className="files-panel__row-actions">
                  <button disabled={busy} onClick={() => void handleDeleteRule(r.path)}>
                    🗑
                  </button>
                </td>
              </tr>
            ))}
            {(status.proxyRules ?? []).length === 0 && (
              <tr>
                <td colSpan={4} className="files-panel__empty">
                  Belum ada aturan per-path.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        <div className="docker-recreate__row" style={{ marginTop: 8 }}>
          <input placeholder="/api" value={rulePath} onChange={(e) => setRulePath(e.target.value)} />
          <input placeholder="http://127.0.0.1:3001" value={ruleTarget} onChange={(e) => setRuleTarget(e.target.value)} />
          <label className="docker-recreate__ro">
            <input type="checkbox" checked={ruleWs} onChange={(e) => setRuleWs(e.target.checked)} />
            ws
          </label>
          <button className="btn btn--sm" disabled={busy || !rulePath.trim() || !ruleTarget.trim()} onClick={() => void handleAddRule()}>
            + Tambah
          </button>
        </div>
      </section>
    </div>
  );
}
