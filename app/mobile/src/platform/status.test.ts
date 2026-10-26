import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash, generateKeyPairSync, sign } from 'node:crypto';
import { stampStatus, statusIsCurrent, monotonicReader, type Service } from './status';
import { verifyStatusProof } from './status-proof';
import { copyIncidentInput, incidentCreate, incidentIssues, incidentPatch, incidentReceipt, type IncidentInput } from './incident-input';
import { MemoryDrafts } from './drafts';
import type { Schemas } from './session';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const service = { id: id(1), evaluated_at: '2026-10-24T12:00:00Z', recheck_at: '2026-10-24T12:00:10Z' } as Service;
test('a status lease consumes the whole network flight and expires exactly at its monotonic boundary', () => {
  const snapshot = stampStatus(service, 1000);
  assert.equal(statusIsCurrent(snapshot, 10999), true); assert.equal(statusIsCurrent(snapshot, 11000), false);
  assert.equal(statusIsCurrent(snapshot, 15000), false, 'late responses cannot restore a green badge');
  const short = stampStatus({ ...service, recheck_at: '2026-10-24T12:00:01Z' }, 1000);
  assert.equal(statusIsCurrent(short, 2000), false, 'maintenance/freshness short deadlines are not rounded up');
  assert.equal(stampStatus({ ...service, recheck_at: '2026-10-24T14:00:00Z' }, 1000).expiresAt, 11000);
});
test('invalid timestamps, clocks, negative leases and clock rollback cannot keep a status live', () => {
  for (const start of [NaN, Infinity, -1]) assert.equal(statusIsCurrent(stampStatus(service, start), 1000), false);
  for (const end of ['invalid', service.evaluated_at, '2026-10-24T11:59:59Z']) assert.equal(statusIsCurrent(stampStatus({ ...service, recheck_at: end }, 1000), 1000), false);
  for (const now of [999, NaN, Infinity, -1]) assert.equal(statusIsCurrent(stampStatus(service, 1000), now), false);
  let now = 1000; const clock = monotonicReader(() => now); assert.equal(clock(), 1000); now = 2000; assert.equal(clock(), 2000);
  now = 1500; assert.ok(Number.isNaN(clock())); now = 5000; assert.ok(Number.isNaN(clock()), 'a poisoned clock needs a new page lifetime');
  assert.ok(Number.isNaN(monotonicReader(() => { throw new Error('unsupported'); })()));
});
const keys = generateKeyPairSync('ed25519');
function evidence(overrides: Partial<Schemas['SignedObservation']> = {}) {
  const raw = { source_revision_id: id(3), sequence: '51', observed_at: '2026-10-24T20:00:00+08:00', healthy: true, latency_ms: 42, ...overrides };
  // Independent Node/OpenSSL signing, non-canonical whitespace and actual UTF-8.
  const bytes = Buffer.from(JSON.stringify(raw, null, 2) + '\n');
  const proof: Schemas['ServiceObservationProof'] = {
    id: id(4), source_id: id(2), source_revision_id: raw.source_revision_id, sequence: raw.sequence,
    observed_at: new Date(raw.observed_at).toISOString(), received_at: '2026-10-24T12:00:01Z', healthy: raw.healthy, latency_ms: raw.latency_ms,
    payload_sha256: createHash('sha256').update(bytes).digest('hex'), applied: true, ignored_reason: '', signed_payload: bytes.toString('base64'),
    signature: sign(null, Buffer.concat([Buffer.from('CP-STATUS-V1\n' + id(2) + '\n'), bytes]), keys.privateKey).toString('base64'),
    public_key: keys.publicKey.export({ type: 'spki', format: 'der' }).subarray(-32).toString('base64'),
  };
  return { proof, row: { ...proof } as Schemas['ServiceObservation'] };
}
test('portable verification accepts the actual raw bytes signed independently by Node/OpenSSL', () => {
  const { proof, row } = evidence(); assert.equal(verifyStatusProof(proof, row), true);
  const original = globalThis.TextEncoder;
  try { Object.defineProperty(globalThis, 'TextEncoder', { value: undefined, configurable: true }); assert.equal(verifyStatusProof(proof, row), true); }
  finally { Object.defineProperty(globalThis, 'TextEncoder', { value: original, configurable: true }); }
});
test('signature, source domain, digest, historical key and selected row are all independently bound', () => {
  const { proof, row } = evidence();
  for (const patch of [
    { id: id(5) }, { source_id: id(6) }, { source_revision_id: id(7) }, { sequence: '52' }, { healthy: false },
    { latency_ms: null }, { observed_at: '2026-10-24T12:00:02Z' }, { received_at: '2026-10-24T12:00:02Z' },
    { applied: false }, { ignored_reason: 'LATE_SEQUENCE' as const }, { payload_sha256: '0'.repeat(64) },
    { signature: Buffer.alloc(64).toString('base64') }, { public_key: Buffer.alloc(32).toString('base64') },
    { signed_payload: Buffer.from('{}').toString('base64') },
  ]) assert.equal(verifyStatusProof({ ...proof, ...patch }, row), false, JSON.stringify(patch));
  const wrongSource = { ...proof, source_id: id(6) }; assert.equal(verifyStatusProof(wrongSource, wrongSource), false, 'changing both response rows cannot change the signed domain');
  const wrongDigest = { ...proof, payload_sha256: '0'.repeat(64) }; assert.equal(verifyStatusProof(wrongDigest, wrongDigest), false);
});
test('proof decoding bounds bytes and rejects unsafe sequences, noncanonical encodings and weak signatures', () => {
  for (const seq of ['0', '01', '9007199254740992']) { const { proof, row } = evidence({ sequence: seq }); assert.equal(verifyStatusProof(proof, row), false); }
  const good = evidence({ sequence: '9007199254740991' }); assert.equal(verifyStatusProof(good.proof, good.row), true);
  const { proof, row } = evidence();
  for (const raw of ['A===', 'A', Buffer.alloc(4097).toString('base64')]) assert.equal(verifyStatusProof({ ...proof, signed_payload: raw }, row), false);
  const weak = { ...proof, public_key: Buffer.from([1, ...new Array(31).fill(0)]).toString('base64'), signature: Buffer.from([1, ...new Array(63).fill(0)]).toString('base64') };
  assert.equal(verifyStatusProof(weak, row), false);
});
const input: IncidentInput = { title: '接口中断', description: '准确影响范围', owner: id(8), severity: 'high', dedup: 'one-intent-only', state: 'open', note: '已登记观测', resolution: '', postmortem: '' };
const current: Schemas['Incident'] = { id: id(9), version: 3, created_at: '2026-10-24T12:00:00Z', updated_at: '2026-10-24T12:00:00Z', title: input.title, service_ids: [id(1)], owner_id: input.owner, dedup_key: input.dedup, severity: input.severity, description: input.description, creator_id: id(8), state: 'open', resolution: '', postmortem: '', resolved_at: null, can_update: true };
test('incident input requires actual affected services and resolution, and never reopens resolved records', () => {
  assert.deepEqual(incidentIssues(input, '人工登记', [id(1)]), []);
  for (const services of [[], [id(1), id(1)], ['not-a-uuid'], Array.from({ length: 21 }, (_, i) => id(i + 1))]) assert.ok(incidentIssues(input, '人工登记', services).some(issue => issue.path === 'services'));
  assert.ok(incidentIssues({ ...input, state: 'resolved' }, '人工核实', [], current).some(issue => issue.path === 'resolution'));
  assert.ok(incidentIssues(input, '追加复盘', [], { ...current, state: 'resolved' }).some(issue => issue.path === 'state'));
  assert.deepEqual(incidentIssues({ ...input, state: 'resolved', resolution: '实际恢复' }, '追加复盘', [], { ...current, state: 'resolved' }), []);
});
test('incident commands preserve exact versions and fixed service sets; receipts cannot claim another intent', () => {
  const ids = [id(1)], create = incidentCreate(input, '人工登记', ids); ids.push(id(2)); assert.deepEqual(create.service_ids, [id(1)]);
  assert.equal(incidentReceipt(current, create), true); assert.equal(incidentReceipt({ ...current, dedup_key: 'another' }, create), false);
  assert.equal(incidentReceipt({ ...current, service_ids: [id(2)] }, create), false);
  const patch = incidentPatch({ ...input, state: 'investigating' }, '继续调查', current.version);
  assert.equal(patch.expected_version, 3); assert.equal('service_ids' in patch, false);
  const updated = { ...current, version: 4, state: 'investigating' as const }; assert.equal(incidentReceipt(updated, patch, current), true);
  for (const wrong of [{ ...updated, version: 3 }, { ...updated, version: 5 }, { ...updated, id: id(10) }, { ...updated, state: 'resolved' as const }]) assert.equal(incidentReceipt(wrong, patch, current), false);
});
test('incident drafts whitelist only input strings, isolate copies and use the existing shared authority and budget', () => {
  const drafts = new MemoryDrafts(() => ({ authenticated: true, epoch: 1 }));
  const authority = { epoch: 1, userID: id(8), authorizationVersion: 1 };
  const target = { kind: 'incident-create' as const, id: 'new-incident', version: 1 };
  const handle = drafts.open(authority, target)!;
  const unsafe = { ...input, service_ids: [id(1)], mfa: '123456', approved: true, access: 'never-retain' };
  assert.equal(handle.write({ reason: '尚未登记', action: 'create', incident: unsafe }), true);
  const copy = handle.read()!.incident!; assert.deepEqual(copy, input); copy.dedup = 'mutated'; assert.equal(handle.read()!.incident!.dedup, input.dedup);
  assert.equal(copyIncidentInput({ ...input, severity: 'other' }), undefined); assert.equal(copyIncidentInput({ ...input, note: '字'.repeat(10001) }), undefined);
  assert.equal(handle.write({ reason: 'invalid', action: 'create', incident: { ...input, title: '字'.repeat(201) } }), false);
  const large = { ...input, description: '字'.repeat(10000), note: '字'.repeat(10000), resolution: '字'.repeat(10000), postmortem: '字'.repeat(10000) };
  for (let n = 0; n < 3; n++) assert.equal(drafts.open(authority, { kind: 'incident-update', id: id(20 + n), version: 1 })!.write({ reason: 'large', action: 'update', incident: large }), true);
  assert.equal(drafts.open(authority, { kind: 'incident-update', id: id(20), version: 1 })!.read(), undefined);
  assert.equal(handle.read(), undefined);
  drafts.open({ ...authority, authorizationVersion: 2 }, target); assert.equal(handle.write({ reason: '', action: 'create', incident: input }), false);
});
