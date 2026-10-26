import { afterEach, describe, expect, it, vi } from 'vitest';
import { confirmedBytes, partBounds, sourceFingerprint, transferUpload, type UploadSession } from './upload';
import type { Schemas } from './api';

const MiB = 1024 * 1024;
const origin = 'https://workspace.example';
const id = '11111111-1111-4111-8111-111111111111';
function upload(source: File, fingerprint: string, overrides: Partial<UploadSession> = {}): UploadSession {
  return { id, version: 2, created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
    folder_id: '22222222-2222-4222-8222-222222222222', file_id: null, base_version_id: null, name: source.name,
    media_type: 'application/octet-stream', source_fingerprint: fingerprint, size_bytes: String(source.size), part_bytes: 5 * MiB,
    part_count: Math.ceil(source.size / (5 * MiB)), confirmed_parts: [], state: 'uploading', storage_epoch: 1,
    expires_at: new Date(Date.now() + 48 * 3600_000).toISOString(), sha256: null, result_version_id: null, result_file_id: null,
    error_code: null, verified_bytes: '0', scan_state: 'pending', scan_result_id: null, scan_waiver_ref: null, ...overrides };
}
function permit(number: number): Schemas['SignedParts']['items'][number] {
  return { part_number: number, method: 'PUT', url: 'https://fixture.oss-cn-hangzhou.aliyuncs.com/drafts/source?signature=transient&part=' + number,
    headers: [], expires_at: new Date(Date.now() + 300_000).toISOString() };
}
function harness(initial: UploadSession, options: { omitLast?: boolean; ackError?: boolean; badPermit?: (value: ReturnType<typeof permit>) => ReturnType<typeof permit> } = {}) {
  let current = structuredClone(initial), reconciliations = 0;
  const cloud = new Set(initial.confirmed_parts);
  const command = vi.fn(async (action: string, body: unknown): Promise<unknown> => {
    const input = body as { part_numbers: number[]; parts: { part_number: number }[] };
    if (action === 'resume') {
      reconciliations++;
      current = { ...current, confirmed_parts: [...cloud].filter(number => !(options.omitLast && reconciliations > 1 && number === current.part_count)) };
      return structuredClone(current);
    }
    if (action === 'sign-parts') return { items: input.part_numbers.map(number => options.badPermit?.(permit(number)) ?? permit(number)) };
    if (action === 'ack-parts') {
      if (options.ackError) throw new Error('结果尚未确认');
      // A receipt is not a cloud-confirmed part until the next ListParts.
      return structuredClone(current);
    }
    if (action === 'finalize') return { ...current, version: current.version + 1, state: 'finalizing' };
    throw new Error('unexpected action');
  });
  const fetcher = vi.fn(async (url: string, init: RequestInit) => {
    init.signal?.throwIfAborted();
    const number = Number(new URL(url).searchParams.get('part'));
    cloud.add(number);
    return new Response(null, { status: 200, headers: { ETag: '"part-' + number + '"' } });
  });
  vi.stubGlobal('fetch', fetcher);
  const controller = new AbortController(), progress = vi.fn(), snapshot = vi.fn(), ensureSession = vi.fn(async () => undefined);
  return { command, fetcher, controller, progress, snapshot, ensureSession,
    run: (source: File) => transferUpload({ upload: initial, source, signal: controller.signal, origin, command: command as <T>(action: 'resume' | 'sign-parts' | 'ack-parts' | 'finalize', body: unknown) => Promise<T>, progress, snapshot, ensureSession }) };
}
afterEach(() => vi.unstubAllGlobals());

describe('bounded resumable upload client', () => {
  it('samples at most 128 KiB for a 120 GB source and computes the last part without allocating it', async () => {
    const slice = vi.fn((start: number, end: number) => new Blob([new Uint8Array(end - start)]));
    const source = { name: 'large.bin', size: 120_000_000_000, lastModified: 123456, slice };
    const fingerprint = await sourceFingerprint(source);
    expect(fingerprint).toMatch(/^web-sample-v1:[0-9a-f]{64}$/);
    expect(slice.mock.calls).toEqual([[0, 65536], [119999934464, 120000000000]]);
    const meta = upload(new File(['x'], 'large.bin'), fingerprint, { size_bytes: '120000000000', part_bytes: 32 * MiB, part_count: 3577 });
    expect(partBounds(meta, 3577)).toEqual({ start: 119990648832, end: 120000000000 });
    expect(() => partBounds(meta, 3578)).toThrow();
    expect(() => confirmedBytes({ ...meta, confirmed_parts: [1, 1] })).toThrow();
    await expect(sourceFingerprint({ ...source, size: 120_000_000_001 })).rejects.toThrow('120 GB');
    expect(slice).toHaveBeenCalledTimes(2);
  });
  it('reconciles before resuming, sends only missing slices, then reconciles before freezing', async () => {
    const source = new File([new Uint8Array(12 * MiB + 3)], 'parts.bin', { lastModified: 456 });
    const current = upload(source, await sourceFingerprint(source), { confirmed_parts: [1] });
    const h = harness(current);
    const result = await h.run(source);
    expect(result.state).toBe('finalizing');
    expect(h.fetcher).toHaveBeenCalledTimes(2);
    expect(h.fetcher.mock.calls.map(call => Number(new URL(call[0]).searchParams.get('part')))).toEqual([2, 3]);
    expect(h.fetcher.mock.calls.map(call => (call[1].body as Blob).size)).toEqual([5 * MiB, 2 * MiB + 3]);
    for (const [, init] of h.fetcher.mock.calls) expect(init).toMatchObject({ credentials: 'omit', redirect: 'error', referrerPolicy: 'no-referrer', cache: 'no-store' });
    expect(h.command.mock.calls.map(call => call[0])).toEqual(['resume', 'sign-parts', 'ack-parts', 'resume', 'finalize']);
    expect(h.progress).toHaveBeenLastCalledWith(source.size, source.size);
    expect(h.snapshot.mock.calls[1][0].confirmed_parts).toEqual([1]);
  });
  it('does not freeze content based solely on client receipts', async () => {
    const source = new File(['small'], 'receipt.bin', { lastModified: 1 });
    const h = harness(upload(source, await sourceFingerprint(source)), { omitLast: true });
    await expect(h.run(source)).rejects.toThrow('云端尚未确认全部分片');
    expect(h.command.mock.calls.some(call => call[0] === 'finalize')).toBe(false);
  });
  it('rejects a reselected source mismatch before signing or cloud access', async () => {
    const source = new File(['first'], 'source.bin', { lastModified: 1 });
    const h = harness(upload(source, await sourceFingerprint(source)));
    await expect(h.run(new File(['other'], source.name, { lastModified: 1 }))).rejects.toThrow('原上传源不一致');
    expect(h.command).not.toHaveBeenCalled(); expect(h.fetcher).not.toHaveBeenCalled();
  });
  it('refuses same-origin addresses, stale permits and credential headers without disclosing signatures', async () => {
    const source = new File(['data'], 'permit.bin', { lastModified: 2 });
    for (const badPermit of [
      (value: ReturnType<typeof permit>) => ({ ...value, url: origin + '/api?signature=do-not-display' }),
      (value: ReturnType<typeof permit>) => ({ ...value, headers: [{ name: 'Authorization', value: 'secret' }] }),
      (value: ReturnType<typeof permit>) => ({ ...value, expires_at: new Date(Date.now() - 1).toISOString() }),
    ]) {
      const h = harness(upload(source, await sourceFingerprint(source)), { badPermit });
      await expect(h.run(source)).rejects.toThrow(/许可|请求头/); expect(h.fetcher).not.toHaveBeenCalled();
    }
  });
  it('stops after an uncertain acknowledgment and never retries or finalizes automatically', async () => {
    const source = new File(['data'], 'uncertain.bin', { lastModified: 3 });
    const h = harness(upload(source, await sourceFingerprint(source)), { ackError: true });
    await expect(h.run(source)).rejects.toThrow('结果尚未确认');
    expect(h.command.mock.calls.filter(call => call[0] === 'ack-parts')).toHaveLength(1);
    expect(h.command.mock.calls.some(call => call[0] === 'finalize')).toBe(false);
  });
  it('aborts active transfer before acknowledging or freezing', async () => {
    const source = new File(['data'], 'pause.bin', { lastModified: 4 });
    const h = harness(upload(source, await sourceFingerprint(source)));
    h.fetcher.mockImplementationOnce(async () => { h.controller.abort(); throw new Error('do-not-display-signed-address'); });
    await expect(h.run(source)).rejects.toThrow();
    expect(h.command.mock.calls.map(call => call[0])).toEqual(['resume', 'sign-parts']);
  });
  it('does not parse or expose raw provider error bodies', async () => {
    const source = new File(['data'], 'error.bin', { lastModified: 5 });
    const h = harness(upload(source, await sourceFingerprint(source)));
    const response = new Response('signature=do-not-display', { status: 403 });
    const read = vi.spyOn(response, 'text'), json = vi.spyOn(response, 'json');
    h.fetcher.mockResolvedValueOnce(response);
    await expect(h.run(source)).rejects.toThrow('云端未确认');
    expect(read).not.toHaveBeenCalled(); expect(json).not.toHaveBeenCalled();
  });
});
