import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { streamDownload, type DownloadGrant, type DownloadSink } from './download';

const identity = { id: '11111111-1111-4111-8111-111111111111', size_bytes: '10', sha256: 'a'.repeat(64) };
const origin = 'https://workspace.example';
function grant(overrides: Partial<DownloadGrant> = {}): DownloadGrant {
  return {
    id: '22222222-2222-4222-8222-222222222222', mode: 'signed_url', url: 'https://storage.example/formal?signature=transient',
    method: 'GET', required_headers: [{ name: 'If-Match', value: '"fixed-source"' }],
    expires_at: new Date(Date.now() + 300_000).toISOString(), permit_expires_at: new Date(Date.now() + 12 * 3600_000).toISOString(),
    file_version_id: identity.id, filename: '合同 原件.bin', size_bytes: identity.size_bytes, sha256: identity.sha256,
    scan_state: 'clean', disposition: 'attachment', max_range_bytes: '4', warnings: ['SIGNED_URL_REVOCATION_DELAY'], ...overrides,
  };
}
function sink(): DownloadSink {
  return { write: vi.fn(async () => undefined), close: vi.fn(async () => undefined), abort: vi.fn(async () => undefined) };
}
function rangeResponse(start: number, end: number, total = 10, etag = '"fixed-source"', actual?: number): Response {
  const size = actual ?? end - start + 1;
  return new Response(new Uint8Array(size), { status: 206, headers: {
    'Content-Length': String(end - start + 1), 'Content-Range': 'bytes ' + start + '-' + end + '/' + total, ETag: etag,
  } });
}
beforeEach(() => vi.stubGlobal('window', new EventTarget()));
afterEach(() => vi.unstubAllGlobals());
describe('bounded fixed-version download streams', () => {
  it('writes ordered ranges and commits once without sending application credentials to cloud storage', async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(rangeResponse(0, 3)).mockResolvedValueOnce(rangeResponse(4, 7)).mockResolvedValueOnce(rangeResponse(8, 9));
    vi.stubGlobal('fetch', fetcher);
    const target = sink(), progress = vi.fn(), renew = vi.fn();
    await streamDownload({ identity, initial: grant(), renew, sink: target, signal: new AbortController().signal, progress, origin });
    expect(fetcher.mock.calls.map(call => call[1].headers.Range)).toEqual(['bytes=0-3', 'bytes=4-7', 'bytes=8-9']);
    for (const call of fetcher.mock.calls) {
      expect(call[1]).toMatchObject({ credentials: 'omit', redirect: 'error', referrerPolicy: 'no-referrer', cache: 'no-store', headers: { 'If-Match': '"fixed-source"' } });
      expect(call[1].headers.Authorization).toBeUndefined();
    }
    expect(target.close).toHaveBeenCalledOnce(); expect(target.abort).not.toHaveBeenCalled();
    expect(progress).toHaveBeenLastCalledWith(10, 10); expect(renew).not.toHaveBeenCalled();
  });
  it('uses the authenticated same-origin proxy and refuses a proxy address on another origin', async () => {
    const current = grant({ mode: 'authenticated_proxy', url: origin + '/v1/downloads/22222222-2222-4222-8222-222222222222/content', required_headers: [], max_range_bytes: '32' });
    const fetcher = vi.fn().mockResolvedValue(rangeResponse(0, 9, 10, '"' + identity.sha256 + '"'));
    vi.stubGlobal('fetch', fetcher);
    await streamDownload({ identity, initial: current, renew: vi.fn(), sink: sink(), signal: new AbortController().signal, progress: vi.fn(), origin });
    expect(fetcher.mock.calls[0][1]).toMatchObject({ credentials: 'same-origin', headers: { 'If-Range': '"' + identity.sha256 + '"' } });
    const target = sink();
    await expect(streamDownload({ identity, initial: { ...current, url: 'https://other.example/v1/downloads/' + current.id + '/content' },
      renew: vi.fn(), sink: target, signal: new AbortController().signal, progress: vi.fn(), origin })).rejects.toThrow('当前工作空间');
    expect(fetcher).toHaveBeenCalledOnce(); expect(target.abort).toHaveBeenCalledOnce();
  });
  it('discards a temporary file for truncated bytes, excessive bytes, wrong ETag or a wrong range response', async () => {
    for (const response of [rangeResponse(0, 3, 10, '"fixed-source"', 3), rangeResponse(0, 3, 10, '"fixed-source"', 5),
      rangeResponse(0, 3, 10, '"different-source"'), rangeResponse(1, 4), new Response(new Uint8Array(4), { status: 200 })]) {
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response));
      const target = sink();
      await expect(streamDownload({ identity, initial: grant(), renew: vi.fn(), sink: target, signal: new AbortController().signal, progress: vi.fn(), origin })).rejects.toThrow();
      expect(target.close).not.toHaveBeenCalled(); expect(target.abort).toHaveBeenCalledOnce();
    }
  });
  it('renews expired signing conditions and rejects a renewed grant for another version before any read', async () => {
    const expired = grant({ expires_at: new Date(Date.now() - 1000).toISOString() });
    const renew = vi.fn().mockResolvedValue(grant({ file_version_id: '33333333-3333-4333-8333-333333333333' }));
    const fetcher = vi.fn(); vi.stubGlobal('fetch', fetcher);
    const target = sink();
    await expect(streamDownload({ identity, initial: expired, renew, sink: target, signal: new AbortController().signal, progress: vi.fn(), origin })).rejects.toThrow('内容版本不一致');
    expect(renew).toHaveBeenCalledOnce(); expect(fetcher).not.toHaveBeenCalled(); expect(target.abort).toHaveBeenCalledOnce();
  });
  it('handles a 120 GB identity with one bounded request and aborts without allocating the full object', async () => {
    const large = { ...identity, size_bytes: '120000000000' }, cancel = new AbortController();
    const fetcher = vi.fn().mockResolvedValue(new Response(new ReadableStream({
      start(controller) { controller.enqueue(new Uint8Array(1024)); },
    }), { status: 206, headers: { 'Content-Length': '33554432', 'Content-Range': 'bytes 0-33554431/120000000000', ETag: '"fixed-source"' } }));
    vi.stubGlobal('fetch', fetcher);
    const target = sink();
    target.write = vi.fn(async () => { cancel.abort(); });
    await expect(streamDownload({ identity: large, initial: grant({ size_bytes: large.size_bytes, max_range_bytes: '33554432' }), renew: vi.fn(),
      sink: target, signal: cancel.signal, progress: vi.fn(), origin })).rejects.toThrow();
    expect(fetcher).toHaveBeenCalledOnce(); expect(fetcher.mock.calls[0][1].headers.Range).toBe('bytes=0-33554431');
    expect(target.write).toHaveBeenCalledOnce(); expect(target.abort).toHaveBeenCalledOnce(); expect(target.close).not.toHaveBeenCalled();
  });
  it('never displays a cloud error containing a signed address', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('provider error https://storage.example?signature=private', { status: 403 })));
    await expect(streamDownload({ identity, initial: grant(), renew: vi.fn(), sink: sink(), signal: new AbortController().signal, progress: vi.fn(), origin }))
      .rejects.toThrow('下载授权已失效');
  });
});
