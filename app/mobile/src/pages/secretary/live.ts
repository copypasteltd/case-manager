import { useEffect, useRef, useState } from 'react';
import { MobileError, Scope, type Page } from '../../platform/session';
import { client } from '../../platform/taro';

// Reload every loaded cursor window, so an older visible message cannot keep
// showing text after its referenced source becomes inaccessible.
export function useMessages<T extends { id: string }>(path: string) {
  const [pages, setPages] = useState<{ path: string; data: Page<T> }[]>([]), [error, setError] = useState<unknown>(), [loading, setLoading] = useState(true), [busy, setBusy] = useState(false), [revision, setRevision] = useState(0);
  const owner = useRef<{ more(): Promise<void> }>();
  useEffect(() => {
    const scope = new Scope(); let loaded: typeof pages = [], flight = false, fatal = false;
    setPages([]); setError(undefined); setLoading(true); setBusy(false);
    async function read(address: string) { const data = await client.request<Page<T>>(address, { scope }); if (!data || !Array.isArray(data.items)) throw new MobileError('INVALID_RESPONSE', '消息列表不完整，请重新读取。'); return { path: address, data }; }
    function failed(cause: unknown) { if (scope.closed) return; setError(cause); if (cause instanceof MobileError && [401, 403, 404, 409, 412].includes(cause.status)) { fatal = true; loaded = []; } else { loaded = []; } setPages([]); }
    async function refresh() {
      if (scope.closed || flight || fatal) return; flight = true;
      try { const result: typeof pages = []; for (const address of loaded.length ? loaded.map(page => page.path) : [path]) result.push(await read(address)); if (!scope.closed) { loaded = result; setPages(result); setError(undefined); } }
      catch (cause) { failed(cause); } finally { flight = false; if (!scope.closed) setLoading(false); }
    }
    async function more() {
      const cursor = loaded.at(-1)?.data.next_cursor; if (!cursor || flight || fatal || scope.closed) return;
      flight = true; setBusy(true);
      try { const result = await read(path + '&cursor=' + encodeURIComponent(cursor)); if (!scope.closed) { loaded = [...loaded, result]; setPages(loaded); setError(undefined); } }
      catch (cause) { failed(cause); } finally { flight = false; if (!scope.closed) setBusy(false); }
    }
    owner.current = { more }; void refresh(); const timer = setInterval(() => void refresh(), 5000);
    return () => { scope.close(); clearInterval(timer); };
  }, [path, revision]);
  const entries = new Map<string, T>(); for (const page of pages) for (const item of page.data.items) if (!entries.has(item.id)) entries.set(item.id, item);
  return { items: [...entries.values()], error, loading, busy, next: pages.at(-1)?.data.next_cursor, more: () => owner.current?.more(), reload: () => setRevision(value => value + 1) };
}
