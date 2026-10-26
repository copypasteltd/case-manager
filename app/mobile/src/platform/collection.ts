import { useCallback, useEffect, useRef, useState } from 'react';
import { Scope, type Page } from './session';
import { client } from './taro';

export function useRecord<T>(path: string | null) {
  const [value, setValue] = useState<T>(), [error, setError] = useState<unknown>(), [loading, setLoading] = useState(Boolean(path));
  useEffect(() => {
    const scope = new Scope(); setValue(undefined); setError(undefined); setLoading(Boolean(path));
    if (path) void client.request<T>(path, { scope }).then(result => { if (!scope.closed) setValue(result); }).catch(cause => { if (!scope.closed) setError(cause); }).finally(() => { if (!scope.closed) setLoading(false); });
    return () => { scope.close(); };
  }, [path]);
  return { value, error, loading };
}

// Mounted only below usePage's current /me and resource checks. Every path or
// refresh owns a new lifetime; late pages cannot append to another collection.
export function useCollection<T>(path: string | null) {
  const current = useRef<Scope>(), flight = useRef(false);
  const [revision, setRevision] = useState(0), [items, setItems] = useState<T[]>([]);
  const [next, setNext] = useState<string | null>(null), [loading, setLoading] = useState(true), [moreBusy, setMoreBusy] = useState(false), [error, setError] = useState<unknown>();
  useEffect(() => {
    const scope = new Scope(); current.current = scope; flight.current = false;
    setItems([]); setNext(null); setError(undefined); setMoreBusy(false); setLoading(Boolean(path));
    if (path) void client.request<Page<T>>(path, { scope }).then(value => {
      if (!scope.closed) { setItems(value.items); setNext(value.next_cursor); }
    }).catch(cause => { if (!scope.closed) setError(cause); }).finally(() => { if (!scope.closed) setLoading(false); });
    return () => { scope.close(); };
  }, [path, revision]);
  const reload = useCallback(() => setRevision(value => value + 1), []);
  async function more() {
    const scope = current.current;
    if (!path || !next || !scope || scope.closed || flight.current || loading) return;
    flight.current = true; setMoreBusy(true); setError(undefined);
    try {
      const value = await client.request<Page<T>>(path + (path.includes('?') ? '&' : '?') + 'cursor=' + encodeURIComponent(next), { scope });
      if (!scope.closed) { setItems(old => [...old, ...value.items]); setNext(value.next_cursor); }
    } catch (cause) { if (!scope.closed) { setError(cause); setItems([]); setNext(null); } }
    finally { if (!scope.closed) { flight.current = false; setMoreBusy(false); } }
  }
  return { items, next, loading, moreBusy, error, more, reload };
}
