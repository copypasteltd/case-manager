import { useEffect, useMemo, useRef, useState } from 'react';
import { MobileError, Scope, type Page } from '../../platform/session';
import { client } from '../../platform/taro';
import { monotonicReader, stampStatus, type Service, type StatusSnapshot } from '../../platform/status';

type Loaded = { path: string; items: StatusSnapshot[]; next: string | null };
function valid(value: Service) {
  if (!value || typeof value.id !== 'string' || typeof value.name !== 'string' || !Array.isArray(value.sources) || !['operational', 'degraded', 'down', 'maintenance', 'unknown'].includes(value.state)) throw new MobileError('INVALID_RESPONSE', '服务状态响应不完整，请重新读取。');
  return value;
}
// Poll loaded pages with their actual cursors; a manual refresh starts a new
// collection. A slow or failed read never extends the previous status lease.
export function useLiveStatus(path: string, single = false) {
  const read = useMemo(() => monotonicReader(() => typeof performance !== 'undefined' && typeof performance.now === 'function' ? performance.now() : NaN), []);
  const [now, setNow] = useState(read), [pages, setPages] = useState<Loaded[]>([]);
  const [error, setError] = useState<unknown>(), [loading, setLoading] = useState(true), [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0), owner = useRef<{ scope: Scope; more(): Promise<void> }>();
  useEffect(() => {
    const scope = new Scope(); let loaded: Loaded[] = [], flight = false, fatal = false;
    setPages([]); setError(undefined); setLoading(true); setBusy(false);
    async function fetchPage(address: string): Promise<Loaded> {
      const started = read();
      if (single) {
        const value = await client.request<Service>(address, { scope });
        if (value?.id !== address.split('/').at(-1)) throw new MobileError('INVALID_RESPONSE', '服务与所选地址不一致，请重新读取。');
        return { path: address, items: [stampStatus(valid(value), started)], next: null };
      }
      const value = await client.request<Page<Service>>(address, { scope });
      if (!value || !Array.isArray(value.items) || value.next_cursor !== null && typeof value.next_cursor !== 'string') throw new MobileError('INVALID_RESPONSE', '服务列表响应不完整，请重新读取。');
      return { path: address, items: value.items.map(item => stampStatus(valid(item), started)), next: value.next_cursor };
    }
    function failed(cause: unknown) {
      if (scope.closed) return;
      setError(cause);
      if (cause instanceof MobileError && [401, 403, 404].includes(cause.status)) { fatal = true; loaded = []; setPages([]); }
    }
    async function refresh() {
      if (scope.closed || flight || fatal) return;
      flight = true;
      try {
        const result: Loaded[] = [];
        for (const address of loaded.length ? loaded.map(page => page.path) : [path]) result.push(await fetchPage(address));
        if (!scope.closed) { loaded = result; setPages(result); setError(undefined); setNow(read()); }
      } catch (cause) { failed(cause); }
      finally { flight = false; if (!scope.closed) setLoading(false); }
    }
    async function more() {
      const cursor = loaded.at(-1)?.next;
      if (!cursor || single || flight || scope.closed || fatal) return;
      flight = true; setBusy(true);
      try {
        const result = await fetchPage(path + (path.includes('?') ? '&' : '?') + 'cursor=' + encodeURIComponent(cursor));
        if (!scope.closed) { loaded = [...loaded, result]; setPages(loaded); setError(undefined); setNow(read()); }
      } catch (cause) { failed(cause); }
      finally { flight = false; if (!scope.closed) setBusy(false); }
    }
    owner.current = { scope, more }; void refresh();
    const poll = setInterval(() => void refresh(), 5000), clock = setInterval(() => { if (!scope.closed) setNow(read()); }, 250);
    return () => { scope.close(); clearInterval(poll); clearInterval(clock); };
  }, [path, single, revision, read]);
  // Repeated first-page updates can overlap an older cursor window. A record
  // appears once, using its latest returned evaluation, without inventing data.
  const records = new Map<string, StatusSnapshot>();
  for (const page of pages) for (const item of page.items) {
    const old = records.get(item.value.id);
    if (!old || Date.parse(item.value.evaluated_at) >= Date.parse(old.value.evaluated_at)) records.set(item.value.id, item);
  }
  return { items: [...records.values()], now, error, loading, busy, next: pages.at(-1)?.next, clockAvailable: Number.isFinite(now), more: () => owner.current?.more(), reload: () => setRevision(value => value + 1) };
}
