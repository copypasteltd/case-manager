import { ApiError, type Schemas } from './api';

export type DownloadGrant = Schemas['DownloadResult'];
export type DownloadIdentity = { id: string; size_bytes: string; sha256: string };
export interface DownloadSink {
  write(data: Uint8Array): Promise<void>;
  close(): Promise<void>;
  abort(): Promise<void>;
}
export type SavePicker = (options: { suggestedName: string }) => Promise<{ createWritable(): Promise<DownloadSink> }>;
export function savePicker(): SavePicker | undefined {
  return (window as Window & { showSaveFilePicker?: SavePicker }).showSaveFilePicker?.bind(window);
}

const maxObject = 120_000_000_000;
const maxRange = 32 * 1024 * 1024;
function count(value: string): number {
  const n = Number(value);
  if (!/^(0|[1-9][0-9]*)$/.test(value) || !Number.isSafeInteger(n) || n < 0 || n > maxObject) throw new Error('下载大小无效，请重新获取文件资料。');
  return n;
}
function assertGrant(grant: DownloadGrant, identity: DownloadIdentity) {
  if (grant.file_version_id !== identity.id || grant.sha256 !== identity.sha256 || grant.size_bytes !== identity.size_bytes ||
    !/^[a-f0-9]{64}$/.test(grant.sha256) || grant.method !== 'GET' || grant.disposition !== 'attachment' ||
    !['clean', 'waived'].includes(grant.scan_state) || !Number.isFinite(Date.parse(grant.expires_at))) {
    throw new Error('下载授权与所选内容版本不一致，请刷新后核对。');
  }
  const limit = count(grant.max_range_bytes);
  if (limit < 1 || limit > maxRange) throw new Error('下载分段限制无效。');
  return limit;
}
function requestScope(grant: DownloadGrant, origin: string) {
  const url = new URL(grant.url, origin);
  if (url.username || url.password || url.hash) throw new Error('下载地址未通过访问检查。');
  const headers: Record<string, string> = {};
  if (grant.mode === 'authenticated_proxy') {
    if (url.origin !== origin || url.pathname !== '/v1/downloads/' + grant.id + '/content' || url.search || grant.required_headers.length) {
      throw new Error('受控下载地址与当前工作空间不一致。');
    }
    headers['If-Range'] = '"' + grant.sha256 + '"';
    return { url, headers, credentials: 'same-origin' as const, etag: headers['If-Range'] };
  }
  if (grant.mode !== 'signed_url' || url.protocol !== 'https:' || url.origin === origin) throw new Error('对象存储下载地址无效。');
  for (const item of grant.required_headers) {
    if (item.name.toLowerCase() !== 'if-match' || headers['If-Match'] || !item.value || /[\r\n]/.test(item.value)) {
      throw new Error('下载请求条件无效。');
    }
    headers['If-Match'] = item.value;
  }
  if (!headers['If-Match']) throw new Error('下载缺少固定版本读取条件。');
  return { url, headers, credentials: 'omit' as const, etag: headers['If-Match'] };
}
async function responseFailure(response: Response, proxy: boolean): Promise<Error> {
  // Cloud errors can contain complete signed request addresses. Do not display,
  // parse or retain them. Only the trusted same-origin API's bounded envelope is
  // eligible to supply a user-facing message.
  if (proxy && response.headers.get('Content-Type')?.startsWith('application/json') && response.body) {
    const reader = response.body.getReader();
    let text = '';
    let bytes = 0;
    const decoder = new TextDecoder();
    try {
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        bytes += value.byteLength;
        if (bytes > 64 * 1024) break;
        text += decoder.decode(value, { stream: true });
      }
      if (bytes <= 64 * 1024) {
        const result = JSON.parse(text + decoder.decode()) as Schemas['Error'];
        if (typeof result.error?.message === 'string' && typeof result.error?.code === 'string') {
          if (response.status === 401) window.dispatchEvent(new Event('cp:session-expired'));
          return new ApiError(response.status, result.error.code, result.error.message, result.meta?.request_id ?? '');
        }
      }
    } catch { /* Only a valid trusted error envelope can be presented. */ }
    finally { await reader.cancel().catch(() => undefined); reader.releaseLock(); }
  } else {
    await response.body?.cancel().catch(() => undefined);
  }
  return new Error(response.status === 403 || response.status === 401
    ? '下载授权已失效，请重新登录或重新获取授权。'
    : '存储暂时无法返回所需内容，请稍后重新下载。');
}

// Each range is streamed into a browser-managed temporary file. Neither this
// function nor the caller creates a whole-object Blob/ArrayBuffer. Only a fully
// received fixed version closes (commits) the selected local file.
export async function streamDownload(options: {
  identity: DownloadIdentity;
  initial: DownloadGrant;
  renew: (signal: AbortSignal) => Promise<DownloadGrant>;
  beforeRange?: (signal: AbortSignal) => Promise<void>;
  sink: DownloadSink;
  signal: AbortSignal;
  progress: (written: number, total: number) => void;
  origin?: string;
}) {
  const { identity, sink, signal, progress } = options;
  const origin = options.origin ?? window.location.origin;
  let grant = options.initial, written = 0;
  let committed = false;
  try {
    const size = count(identity.size_bytes);
    assertGrant(grant, identity);
    while (written < size) {
      signal.throwIfAborted();
      await options.beforeRange?.(signal);
      signal.throwIfAborted();
      if (Date.parse(grant.expires_at) < Date.now() + 30_000) {
        grant = await options.renew(signal);
        assertGrant(grant, identity);
        if (Date.parse(grant.expires_at) < Date.now() + 1000) throw new Error('新的下载授权已到期，请重新申请。');
      }
      const limit = assertGrant(grant, identity);
      const scope = requestScope(grant, origin);
      const end = Math.min(size - 1, written + limit - 1);
      const expected = end - written + 1;
      const start = written;
      const rangeContext = new AbortController();
      const userAbort = () => rangeContext.abort(signal.reason);
      signal.addEventListener('abort', userAbort, { once: true });
      const deadline = setTimeout(() => rangeContext.abort(new DOMException('范围读取超时', 'TimeoutError')), 10 * 60_000);
      let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
      try {
        let response: Response;
        try {
          response = await fetch(scope.url.href, {
            method: 'GET', credentials: scope.credentials, mode: 'cors', redirect: 'error', cache: 'no-store', referrerPolicy: 'no-referrer',
            headers: { ...scope.headers, Range: 'bytes=' + start + '-' + end }, signal: rangeContext.signal,
          });
        } catch {
          signal.throwIfAborted();
          throw new Error('分段读取中断，请检查网络后重新下载。');
        }
        if (!response.ok) throw await responseFailure(response, grant.mode === 'authenticated_proxy');
        if (response.status !== 206 || response.headers.get('Content-Range') !== 'bytes ' + start + '-' + end + '/' + size ||
          response.headers.get('ETag') !== scope.etag || response.headers.get('Content-Length') !== String(expected) ||
          (response.headers.get('Content-Encoding') && response.headers.get('Content-Encoding') !== 'identity') || !response.body) {
          await response.body?.cancel().catch(() => undefined);
          throw new Error('存储返回的版本或读取范围与下载授权不一致。');
        }
        reader = response.body.getReader();
        let received = 0;
        for (;;) {
          signal.throwIfAborted();
          let chunk: ReadableStreamReadResult<Uint8Array>;
          try { chunk = await reader.read(); }
          catch { signal.throwIfAborted(); throw new Error('分段内容未完整读取，请重新下载。'); }
          if (chunk.done) break;
          if (chunk.value.byteLength > expected - received) throw new Error('存储返回了超出授权范围的内容，下载已停止。');
          await sink.write(chunk.value);
          received += chunk.value.byteLength;
          written += chunk.value.byteLength;
          progress(written, size);
        }
        if (received !== expected) throw new Error('分段内容不完整，未保存本次文件。');
      } finally {
        clearTimeout(deadline);
        signal.removeEventListener('abort', userAbort);
        rangeContext.abort();
        await reader?.cancel().catch(() => undefined);
        reader?.releaseLock();
      }
    }
    signal.throwIfAborted();
    await sink.close();
    committed = true;
    progress(size, size);
  } finally {
    if (!committed) await sink.abort().catch(() => undefined);
  }
}
