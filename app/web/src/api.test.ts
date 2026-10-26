import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError, ensureDownloadSession } from './api';

const success = (data: unknown) => new Response(JSON.stringify({ data, meta: { request_id: 'test-request', server_time: '2026-10-23T00:00:00Z' } }), { status: 200 });
const unauthorized = () => new Response(JSON.stringify({ error: { code: 'UNAUTHENTICATED', message: '请登录', retryable: false }, meta: { request_id: 'denied', server_time: '2026-10-23T00:00:00Z' } }), { status: 401 });
beforeEach(() => {
  vi.stubGlobal('document', { cookie: 'cp_csrf=test-csrf' });
  vi.stubGlobal('window', new EventTarget());
});
afterEach(() => vi.unstubAllGlobals());
describe('session and uncertain command handling', () => {
  it('refreshes a failed read once and retries that read', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(unauthorized()).mockResolvedValueOnce(success({ step: 'authenticated' })).mockResolvedValueOnce(success({ name: 'visible' }));
    vi.stubGlobal('fetch', fetch);
    expect(await api('/projects')).toEqual({ name: 'visible' });
    expect(fetch.mock.calls.map(call => call[0])).toEqual(['/v1/projects', '/v1/auth/refresh', '/v1/projects']);
    expect(fetch.mock.calls[1][1].headers['X-CSRF-Token']).toBe('test-csrf');
  });
  it('does not resend a command after losing the connection', async () => {
    const fetch = vi.fn().mockRejectedValue(new TypeError('network interrupted'));
    vi.stubGlobal('fetch', fetch);
    await expect(api('/projects', { method: 'POST', body: { name: 'P' }, key: 'stable-command-key' })).rejects.toMatchObject({ code: 'RESULT_UNCONFIRMED', status: 0 });
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0][1].headers['Idempotency-Key']).toBe('stable-command-key');
  });
  it('does not refresh and automatically resend an unauthorized write', async () => {
    const fetch = vi.fn().mockResolvedValue(unauthorized());
    vi.stubGlobal('fetch', fetch);
    const expired = vi.fn(); window.addEventListener('cp:session-expired', expired);
    await expect(api('/projects', { method: 'POST', body: {} })).rejects.toBeInstanceOf(ApiError);
    expect(fetch).toHaveBeenCalledTimes(1); expect(expired).toHaveBeenCalledOnce();
  });
  it('preserves server conflict details and request evidence', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: 'VERSION_CONFLICT', message: '资料已更新', retryable: false, details: [{ field: 'version', code: 'STALE', safe_message: '请比较资料' }] }, meta: { request_id: 'conflict-evidence' } }), { status: 409 })));
    await expect(api('/projects/p', { method: 'PATCH', body: {} })).rejects.toMatchObject({ code: 'VERSION_CONFLICT', requestID: 'conflict-evidence', details: [{ field: 'version', code: 'STALE', safe_message: '请比较资料' }] });
  });
  it('renews an expiring download session between ranges and shares concurrent checks', async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(success({ session_expires_at: new Date(Date.now() + 60_000).toISOString() }))
      .mockResolvedValueOnce(success({ status: 'authenticated' }))
      .mockResolvedValueOnce(success({ session_expires_at: new Date(Date.now() + 15 * 60_000).toISOString() }));
    vi.stubGlobal('fetch', fetcher);
    await Promise.all([ensureDownloadSession(), ensureDownloadSession()]);
    expect(fetcher.mock.calls.map(call => call[0])).toEqual(['/v1/me', '/v1/auth/refresh', '/v1/me']);
  });
  it('retries a delayed stale read after another request refreshed without rotating the new session again', async () => {
    let stale!: (value: Response) => void;
    const pending = new Promise<Response>(resolve => { stale = resolve; });
    const fetcher = vi.fn().mockReturnValueOnce(pending).mockResolvedValueOnce(unauthorized())
      .mockResolvedValueOnce(success({ status: 'authenticated' })).mockResolvedValueOnce(success({ name: 'second' }))
      .mockResolvedValueOnce(success({ name: 'first' }));
    vi.stubGlobal('fetch', fetcher);
    const first = api('/first');
    expect(await api('/second')).toEqual({ name: 'second' });
    stale(unauthorized());
    expect(await first).toEqual({ name: 'first' });
    expect(fetcher.mock.calls.filter(call => call[0] === '/v1/auth/refresh')).toHaveLength(1);
  });
});
