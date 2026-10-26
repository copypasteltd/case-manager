import { test } from 'node:test';
import assert from 'node:assert/strict';
import { MobileClient, type Schemas, type WireRequest, type WireResponse } from './session';
import { UploadTransferController, uploadedBytes, type NativeUploadPort, type NativeUploadSource, type Upload } from './upload-transfer';

const id = '11111111-1111-4111-8111-111111111111', handle = '22222222-2222-4222-8222-222222222222';
const owner = '33333333-3333-4333-8333-333333333333', run = '44444444-4444-4444-8444-444444444444';
const folder = '55555555-5555-4555-8555-555555555555', fingerprint = 'android-saf-v1:' + 'a'.repeat(64), MiB = 1024 * 1024;
const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T;
const response = (data: unknown): WireResponse => ({ status: 200, data: { data, meta: { request_id: 'native-upload-test-request' } } });
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done; }); return { promise, resolve }; }
function initial(): Upload {
  return { id, version: 1, created_at: '2026-10-26T00:00:00Z', updated_at: '2026-10-26T00:00:00Z', folder_id: folder,
    file_id: null, base_version_id: null, name: 'archive.bin', media_type: 'application/octet-stream', source_fingerprint: fingerprint,
    error_code: null, verified_bytes: '0', scan_state: 'pending', scan_result_id: null, scan_waiver_ref: null,
    size_bytes: String(5 * MiB + 3), part_bytes: 5 * MiB, part_count: 2, confirmed_parts: [], state: 'uploading', storage_epoch: 1,
    expires_at: '2026-10-28T00:00:00Z', sha256: null, result_version_id: null, result_file_id: null };
}
async function harness(existing = true) {
  const model = initial(), cloud = new Set<number>(), requests: WireRequest[] = [], puts: number[] = [], finished: string[] = [];
  const state = { now: Date.parse('2026-10-26T00:00:00Z'), actor: owner, failAck: false, failCreate: false, missingFinal: false, resumes: 0,
    rotateAfterPart: false, partGate: null as ReturnType<typeof deferred<void>> | null,
    resumeTransform: (value: Upload) => value, permitTransform: (value: Schemas['SignedParts']) => value };
  const source: NativeUploadSource = { handle, name: model.name, media_type: model.media_type, size_bytes: model.size_bytes,
    fingerprint, upload_id: existing ? id : '', create_key: 'android-upload:original-durable-intent',
    create_body: JSON.stringify({ folder_id: folder, name: model.name, size_bytes: model.size_bytes, media_type: model.media_type, source_fingerprint: fingerprint }), state: 'prepared', has_source: true, prepared_at_ms: Date.now() };
  let nonce = 0;
  const client = new MobileClient(input => {
    requests.push(clone(input));
    const promise = Promise.resolve().then(() => {
      if (input.path === '/auth/login' || input.path === '/auth/refresh') return response({ step: 'authenticated', access_token: 'test-access-' + (++nonce), refresh_token: 'test-refresh-' + nonce, expires_at: new Date(state.now + 90_000).toISOString() });
      if (input.path === '/me') return response({ user: { id: state.actor } });
      if (input.path === '/uploads' && input.method === 'POST') { if (state.failCreate) { state.failCreate = false; throw new Error('lost create response'); } return response(clone(model)); }
      if (input.path === '/uploads/' + id && input.method === 'GET') return response(clone(model));
      const action = input.path.split('/').at(-1), body = input.body as { expected_version: number; part_numbers?: number[] };
      assert.equal(body.expected_version, model.version);
      if (action === 'resume') {
        state.resumes++; model.version++; model.confirmed_parts = state.missingFinal && state.resumes > 1 ? [] : [...cloud];
        return response(state.resumeTransform(clone(model)));
      }
      if (action === 'sign-parts') return response(state.permitTransform({ items: body.part_numbers!.map(part => ({ part_number: part, method: 'PUT', url: 'https://bucket.aliyuncs.com/draft?signature=memory-only', headers: [], expires_at: new Date(state.now + 300_000).toISOString() })) }));
      if (action === 'ack-parts') { model.version++; if (state.failAck) { state.failAck = false; throw new Error('unknown acknowledgement'); } return response(clone(model)); }
      if (action === 'finalize') { model.version++; model.state = 'finalizing'; return response(clone(model)); }
      throw new Error('Unexpected API action');
    });
    return { promise, abort() {} };
  }, 'android', () => state.now);
  await client.login('test-user', 'test-password');
  const native: NativeUploadPort = {
    async scope(value) { assert.equal(value, owner); }, async list() { return [clone(source)]; },
    async begin(value) { assert.equal(value, handle); return run; },
    async bind(value, upload) { assert.equal(value, handle); assert.equal(upload.source_fingerprint, fingerprint); source.upload_id = upload.id; return clone(source); },
    async touch() {},
    async transfer(value, running, part, permit) {
      assert.equal(value, handle); assert.equal(running, run); assert.equal(permit.part_number, part);
      assert.ok(!JSON.stringify(permit).includes('test-access')); assert.ok(!JSON.stringify(permit).includes('test-refresh'));
      puts.push(part); cloud.add(part); if (state.partGate) await state.partGate.promise;
      if (state.rotateAfterPart) { state.now += 70_000; state.rotateAfterPart = false; }
      return { part_number: part, etag: '"receipt-' + part + '"', size_bytes: String(Math.min(model.part_bytes, Number(model.size_bytes) - (part - 1) * model.part_bytes)) };
    },
    async verifySource() {}, async finish(_handle, _run, phase) { finished.push(phase); }, pause() {},
  };
  const controller = new UploadTransferController(client, native);
  return { client, native, controller, state, source, model, requests, puts, cloud, finished };
}
const actions = (h: Awaited<ReturnType<typeof harness>>, action: string) => h.requests.filter(request => request.path.endsWith('/' + action));
const tick = () => new Promise<void>(done => setImmediate(done));

test('120 GB layout and last part retain exact byte counts without reading file content', () => {
  const upload = { ...initial(), size_bytes: '120000000000', part_bytes: 32 * MiB, part_count: 3577, confirmed_parts: [1, 3577] };
  assert.equal(uploadedBytes(upload), 42_905_600); assert.throws(() => uploadedBytes({ ...upload, confirmed_parts: [1, 1] }));
  assert.throws(() => uploadedBytes({ ...upload, size_bytes: '120000000001' })); assert.throws(() => uploadedBytes({ ...upload, part_count: 3576 }));
});
test('recovery reconciles cloud parts, sends only missing data and finalizes without committing', async () => {
  const h = await harness(); h.cloud.add(1); const result = await h.controller.start(handle, owner);
  assert.deepEqual(h.puts, [2]); assert.equal(h.state.resumes, 2); assert.equal(result.state, 'finalizing');
  assert.equal(actions(h, 'commit').length, 0); assert.equal(actions(h, 'finalize').length, 1);
  assert.equal(h.controller.snapshot().confirmed, Number(h.source.size_bytes)); assert.deepEqual(h.finished, ['awaiting_verification']);
});
test('unknown acknowledgement stops immediately; explicit continuation reconciles an unacknowledged cloud part', async () => {
  const h = await harness(); h.state.failAck = true;
  await assert.rejects(h.controller.start(handle, owner), { code: 'RESULT_UNCONFIRMED' });
  assert.deepEqual(h.puts, [1]); assert.equal(actions(h, 'finalize').length, 0); assert.equal(actions(h, 'ack-parts').length, 1);
  const result = await h.controller.start(handle, owner); assert.equal(result.state, 'finalizing'); assert.deepEqual(h.puts, [1, 2]);
});
test('an unknown create is never automatically replayed; manual reconciliation uses the same durable intent', async () => {
  const h = await harness(false); h.state.failCreate = true;
  await assert.rejects(h.controller.start(handle, owner), { code: 'RESULT_UNCONFIRMED' }); assert.equal(h.puts.length, 0); assert.equal(actions(h, 'uploads').length, 1);
  await h.controller.start(handle, owner);
  const calls = actions(h, 'uploads'); assert.equal(calls.length, 2); assert.equal(calls[0].headers['Idempotency-Key'], calls[1].headers['Idempotency-Key']); assert.deepEqual(calls[0].body, calls[1].body);
});
test('expired preparation and a changed creation target cannot silently establish a replacement intent', async () => {
  const old = await harness(false); old.source.prepared_at_ms = Date.now() - 48 * 60 * 60_000 - 1;
  await assert.rejects(old.controller.start(handle, owner), { code: 'UPLOAD_CHANGED' }); assert.equal(actions(old, 'uploads').length, 0);
  const changed = await harness(false); changed.model.folder_id = owner;
  await assert.rejects(changed.controller.start(handle, owner), { code: 'UPLOAD_CHANGED' }); assert.equal(changed.puts.length, 0);
});
test('a changed source layout, storage epoch or target prevents any native transfer', async () => {
  for (const change of [{ part_bytes: 32 * MiB, part_count: 1 }, { storage_epoch: 2 }, { folder_id: owner }, { source_fingerprint: 'changed-source' }]) {
    const h = await harness(); h.state.resumeTransform = value => ({ ...value, ...change });
    await assert.rejects(h.controller.start(handle, owner), { code: 'UPLOAD_CHANGED' }); assert.equal(h.puts.length, 0);
  }
});
test('wrong or incomplete permits stop before native network I/O', async () => {
  for (const change of [() => ({ items: [] }), (value: Schemas['SignedParts']) => ({ items: [{ ...value.items[0], part_number: 9 }] })]) {
    const h = await harness(); h.state.permitTransform = change;
    await assert.rejects(h.controller.start(handle, owner), { code: 'UPLOAD_CHANGED' }); assert.equal(h.puts.length, 0);
  }
});
test('cancellation during native I/O rejects late receipt and sends no acknowledgement', async () => {
  const h = await harness(); h.state.partGate = deferred<void>(); const call = h.controller.start(handle, owner);
  while (!h.puts.length) await tick();
  const rejected = assert.rejects(call, { code: 'UPLOAD_PAUSED' }); h.controller.pause(); h.state.partGate.resolve(); await rejected;
  assert.equal(actions(h, 'ack-parts').length, 0); assert.equal(actions(h, 'finalize').length, 0); assert.equal(h.finished.at(-1), 'paused');
});
test('system notification pause affects only the active native run', async () => {
  const h = await harness(); h.state.partGate = deferred<void>(); const call = h.controller.start(handle, owner);
  while (!h.puts.length) await tick();
  h.controller.event({ handle, run: owner, state: 'paused', sent_bytes: '0', confirmed_bytes: '0', size_bytes: h.model.size_bytes });
  assert.equal(h.controller.snapshot().phase, 'transferring');
  const rejected = assert.rejects(call, { code: 'UPLOAD_PAUSED' });
  h.controller.event({ handle, run, state: 'paused', sent_bytes: '0', confirmed_bytes: '0', size_bytes: h.model.size_bytes });
  h.state.partGate.resolve(); await rejected; assert.equal(actions(h, 'ack-parts').length, 0);
});
test('logout clears visible upload details and prevents late native acknowledgement', async () => {
  const h = await harness(); h.state.partGate = deferred<void>(); const call = h.controller.start(handle, owner);
  while (!h.puts.length) await tick();
  const rejected = assert.rejects(call, { code: 'UPLOAD_PAUSED' }); h.client.clear();
  assert.equal(h.controller.snapshot().upload, null); assert.equal(h.controller.snapshot().handle, null);
  h.state.partGate.resolve(); await rejected; assert.equal(h.controller.snapshot().upload, null); assert.equal(actions(h, 'ack-parts').length, 0);
});
test('finalize requires authoritative cloud reconciliation of the complete layout', async () => {
  const h = await harness(); h.state.missingFinal = true;
  await assert.rejects(h.controller.start(handle, owner), { code: 'UPLOAD_CHANGED' }); assert.deepEqual(h.puts, [1, 2]); assert.equal(actions(h, 'finalize').length, 0);
});
test('memory-only credential rotation continues through the existing client and never reaches the provider', async () => {
  const h = await harness(); h.state.rotateAfterPart = true; await h.controller.start(handle, owner);
  assert.equal(actions(h, 'refresh').length, 1);
  const sign = actions(h, 'sign-parts'); assert.equal(sign[0].headers.Authorization, 'Bearer test-access-1'); assert.equal(sign[1].headers.Authorization, 'Bearer test-access-2');
  const keys = h.requests.filter(value => value.method === 'POST' && value.headers['Idempotency-Key']).map(value => value.headers['Idempotency-Key']); assert.equal(new Set(keys).size, keys.length);
});
test('already prepared server results and mismatched identity cannot trigger data transmission', async () => {
  const h = await harness(); h.model.state = 'ready'; await h.controller.start(handle, owner); assert.equal(h.puts.length, 0); assert.equal(actions(h, 'commit').length, 0);
  const other = await harness(); other.state.actor = folder;
  await assert.rejects(other.controller.start(handle, owner), { code: 'UPLOAD_CHANGED' }); assert.equal(other.puts.length, 0); assert.equal(actions(other, id).length, 0);
});
