import type { components } from '@copypaste/contracts';

export type Schemas = components['schemas'];
export type Project = Schemas['Project'];
export type Task = Schemas['Task'];
export type Me = Schemas['Me'];
type Meta = Schemas['Meta'];
export type Page<T> = { items: T[]; next_cursor: string | null };
export class ApiError extends Error {
  constructor(readonly status: number, readonly code: string, message: string, readonly requestID: string, readonly details: Schemas['Error']['error']['details'] = []) { super(message); }
}
function csrf(): string {
  const value = document.cookie.split('; ').find(item => item.startsWith('cp_csrf='));
  return value ? decodeURIComponent(value.slice('cp_csrf='.length)) : '';
}
let refreshInProgress: Promise<boolean> | null = null;
let sessionEpoch = 0;
async function refreshSession(): Promise<boolean> {
  if (!refreshInProgress) {
    refreshInProgress = fetch('/v1/auth/refresh', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: '{}' })
      .then(result => { if (result.ok) sessionEpoch++; return result.ok; }).catch(() => false).finally(() => { refreshInProgress = null; });
  }
  return refreshInProgress;
}
export async function api<T>(path: string, options: { method?: 'GET' | 'POST' | 'PATCH'; body?: unknown; key?: string; signal?: AbortSignal } = {}, retried = false): Promise<T> {
  const method = options.method ?? 'GET';
  const sentEpoch = sessionEpoch;
  const headers: Record<string, string> = {};
  if (method !== 'GET') {
    headers['Content-Type'] = 'application/json'; headers['X-CSRF-Token'] = csrf();
    if (!path.startsWith('/auth/')) headers['Idempotency-Key'] = options.key ?? crypto.randomUUID();
  }
  let response: Response;
  try {
    response = await fetch('/v1' + path, { method, credentials: 'same-origin', headers, body: method === 'GET' ? undefined : JSON.stringify(options.body ?? {}), signal: options.signal });
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error;
    throw new ApiError(0, 'RESULT_UNCONFIRMED', method === 'GET' ? '无法连接服务，请稍后重试。' : '连接中断，操作结果尚未确认。请刷新资料核对后再操作。', '');
  }
  if (response.status === 401 && !retried && !path.startsWith('/auth/') && method === 'GET' &&
    (sessionEpoch !== sentEpoch || await refreshSession())) return api<T>(path, options, true);
  let value: { data?: T; meta?: Meta; error?: Schemas['Error']['error'] };
  try { value = await response.json(); } catch { throw new ApiError(response.status, 'INVALID_RESPONSE', '服务响应异常，请保留请求编号。', response.headers.get('X-Request-ID') ?? ''); }
  if (!response.ok || value.error) {
    if (response.status === 401 && !path.startsWith('/auth/')) window.dispatchEvent(new Event('cp:session-expired'));
    throw new ApiError(response.status, value.error?.code ?? 'REQUEST_FAILED', value.error?.message ?? '操作未能完成。', value.meta?.request_id ?? '', value.error?.details);
  }
  return value.data as T;
}

let downloadSessionInProgress: Promise<void> | null = null;
// Refresh between ranges with enough life for the proxy's maximum two-minute
// request. A shared promise avoids rotating one device under another download.
export async function ensureDownloadSession(signal?: AbortSignal): Promise<void> {
  if (!downloadSessionInProgress) {
    downloadSessionInProgress = (async () => {
      const current = await api<Me>('/me', { signal });
      const expiry = Date.parse(current.session_expires_at);
      if (!Number.isFinite(expiry)) throw new ApiError(500, 'SESSION_EXPIRY_INVALID', '暂时无法确认下载会话，请重新登录。', '');
      if (expiry <= Date.now() + 3 * 60_000) {
        if (!await refreshSession()) {
          window.dispatchEvent(new Event('cp:session-expired'));
          throw new ApiError(401, 'UNAUTHENTICATED', '会话已到期，请重新登录后下载。', '');
        }
        await api<Me>('/me', { signal });
      }
    })().finally(() => { downloadSessionInProgress = null; });
  }
  await downloadSessionInProgress;
  signal?.throwIfAborted();
}
