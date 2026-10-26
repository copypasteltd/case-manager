import test from 'node:test';
import assert from 'node:assert/strict';
import type { Schemas } from './session';
import { archivePayload, archiveProof, archiveReceipt, closeoutLabels, currentArchives, currentCloseout,
  projectActions, projectStateReceipt, projectTransition, transitionState } from './project-closeout';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const me = { user: { id: id(2) }, actions: ['project.update', 'project.archive', 'project.reopen'] } as Schemas['Me'];
const members = { items: [], memberships: [{ user_id: me.user.id, project_role: 'manager' }] } as Schemas['ProjectMembers'];
const project = { id: id(1), version: 7, name: 'Actual project', project_no: 'CP-1', owner_id: me.user.id,
  state: 'closing', scope: 'Fixed original scope', type: 'internal', classification: 'project_restricted' } as Schemas['Project'];
function basis(): Schemas['ProjectCloseout'] {
  return { project: { ...project }, visibility: 'complete', as_of: '2028-02-29T01:00:00Z', fingerprint: 'a'.repeat(32), can_close: false, can_archive: true,
    groups: Object.keys(closeoutLabels).map(kind => ({ kind, visibility: 'complete', count: 0, open_count: 0, items: [] })) } as Schemas['ProjectCloseout'];
}
const notes = { summary: '\u0085 Actual delivery \u3000', lessons: 'Actual lessons', followUp: 'Explicitly no remaining follow-up', reason: ' Checked current business ' };
const file = { file_id: id(20), version_id: id(21), seq: 3, name: 'Actual retrospective.txt', sha256: 'b'.repeat(64), size_bytes: '123456789123456789' };
function receipt(b = basis(), files = [] as typeof file[]) {
  const body = archivePayload(b, members, me, notes, files, true, true, true);
  const value: Schemas['ProjectArchiveReceipt'] = { project: { ...b.project, version: b.project.version + 1, state: 'archived' },
    archive: { id: id(30), project_id: b.project.id, project_version: b.project.version + 1, project_name: b.project.name, fingerprint: body.expected_fingerprint,
      summary: body.summary, lessons_learned: body.lessons_learned, follow_up: body.follow_up, reason: body.reason, actor_id: me.user.id, actor_name: 'Current member name',
      business_verified: true, financial_verified: true, current_basis_state: 'current', file_version_ids: body.file_version_ids,
      attachments: files.map(f => ({ ...f, media_type: 'text/plain', scan_state: 'clean', scan_waiver_ref: null })), groups: structuredClone(b.groups), created_at: '2028-02-29T01:01:00Z' } };
  return { body, value };
}
const copy = <T>(value: T): T => structuredClone(value);

test('thirteen complete categories and exact counts are required, including resource identity and actual budget revisions', () => {
  const b = basis(); b.groups[0] = { kind: 'tasks', visibility: 'complete', count: 1, open_count: 0, items: [{ kind: 'tasks', id: id(40), version: 9, title: 'Actual task', state: 'done', blocking: false }] };
  b.groups[6] = { kind: 'budgets', visibility: 'complete', count: 1, open_count: 0, items: [{ kind: 'budgets', id: id(41), version: 6, title: 'Actual budget revision', state: 'effective', blocking: false }] };
  assert.equal(currentCloseout(b, project.id), b);
  assert.throws(() => currentCloseout(b, id(99)));
  for (const mutate of [(v: any) => v.groups.pop(), (v: any) => v.groups.push(v.groups[0]), (v: any) => v.groups[0].count++,
    (v: any) => v.groups[0].items[0].version = 0, (v: any) => v.groups[0].items[0].kind = 'budgets', (v: any) => v.groups[0].open_count = 1]) {
    const bad = copy(b); mutate(bad); assert.throws(() => currentCloseout(bad, project.id));
  }
});

test('restricted categories retain null counts, no items and no complete fingerprint or write affordance', () => {
  const b = basis(); b.visibility = 'restricted'; b.fingerprint = null; b.can_archive = false;
  b.groups[4] = { kind: 'issues', visibility: 'restricted', count: null, open_count: null, items: [] };
  assert.equal(currentCloseout(b, project.id).groups[4].count, null);
  for (const mutate of [(v: any) => v.groups[4].count = 0, (v: any) => v.fingerprint = 'a'.repeat(32), (v: any) => v.can_close = true]) {
    const bad = copy(b); mutate(bad); assert.throws(() => currentCloseout(bad, project.id));
  }
  assert.throws(() => archivePayload(b, members, me, notes, [], true, true, true));
});

test('project lifecycle preserves original permissions, state targets and the independent archive boundary', () => {
  const b = basis(); b.project.state = 'active'; b.can_archive = false; b.can_close = true;
  assert.deepEqual(projectActions(b, members, me).map(a => a.action), ['hold', 'close', 'cancel']);
  assert.deepEqual(projectTransition(b, members, me, 'close', ' Actual close '), { expected_version: 7, action: 'close', reason: 'Actual close' });
  assert.equal(transitionState('close'), 'closing'); assert.equal(transitionState('reopen'), 'active');
  assert.throws(() => projectTransition(b, members, me, 'archive', 'No bypass'));
  assert.throws(() => projectTransition(b, members, me, 'reopen', 'Wrong stage'));
  assert.throws(() => projectTransition(b, members, me, 'close', ' '));
  b.project.state = 'draft'; b.can_close = false;
  assert.deepEqual(projectActions(b, members, me).map(a => a.action), ['activate', 'cancel']);
  b.project.state = 'archived'; assert.deepEqual(projectActions(b, members, me).map(a => a.action), ['reopen']);
});

test('viewer and missing business actions cannot transition or archive even when a stale capability says yes', () => {
  const viewer = { ...members, memberships: [{ user_id: me.user.id, project_role: 'viewer' as const, created_at: '2028-01-01T00:00:00Z' }] } as Schemas['ProjectMembers'];
  assert.deepEqual(projectActions(basis(), viewer, me), []);
  assert.throws(() => archivePayload(basis(), viewer, me, notes, [], true, true, true));
  assert.throws(() => archivePayload(basis(), members, { ...me, actions: ['project.update'] }, notes, [], true, true, true));
  const b = basis(); b.can_archive = false; b.project.state = 'archived';
  assert.deepEqual(projectActions(b, members, { ...me, actions: ['project.update'] }), []);
});

test('a project transition receipt must preserve the exact header and advance only the intended state and version', () => {
  const b = basis().project, actual = { ...b, version: b.version + 1, state: 'active' as const };
  assert.equal(projectStateReceipt(actual, b, 'active'), true);
  for (const change of [{ id: id(99) }, { version: b.version }, { state: 'archived' }, { scope: 'Silently changed' }, { owner_id: id(99) }, { classification: 'internal' }]) {
    assert.equal(projectStateReceipt({ ...actual, ...change }, b, 'active'), false);
  }
});

test('archive inputs bind exact version and fingerprint, require three notes, two explicit attestations and fresh MFA', () => {
  const b = basis(), input = archivePayload(b, members, me, notes, [], true, true, true);
  assert.equal(input.expected_version, 7); assert.equal(input.expected_fingerprint, b.fingerprint); assert.equal(input.summary, 'Actual delivery');
  assert.deepEqual(input.file_version_ids, []); assert.equal('mfa' in input, false);
  for (const key of ['summary', 'lessons', 'followUp', 'reason']) assert.throws(() => archivePayload(b, members, me, { ...notes, [key]: ' ' }, [], true, true, true));
  assert.throws(() => archivePayload(b, members, me, { ...notes, lessons: 'x'.repeat(20001) }, [], true, true, true));
  assert.throws(() => archivePayload(b, members, me, { ...notes, reason: 'x'.repeat(201) }, [], true, true, true));
  for (const flags of [[false, true, true], [true, false, true], [true, true, false]]) assert.throws(() => archivePayload(b, members, me, notes, [], flags[0], flags[1], flags[2]));
});

test('optional archive attachments use distinct actual fixed versions with a maximum of twenty', () => {
  assert.deepEqual(archivePayload(basis(), members, me, notes, [file], true, true, true).file_version_ids, [file.version_id]);
  assert.throws(() => archivePayload(basis(), members, me, notes, [file, file], true, true, true));
  assert.throws(() => archivePayload(basis(), members, me, notes, [{ ...file, version_id: 'arbitrary' }], true, true, true));
  assert.throws(() => archivePayload(basis(), members, me, notes, Array.from({ length: 21 }, (_, i) => ({ ...file, version_id: id(i + 100) })), true, true, true));
});

test('full archive receipts reconcile original project, notes, actor, fingerprint, groups and new archive revision', () => {
  const b = basis(), { body, value } = receipt(b);
  assert.equal(archiveReceipt(value, b, body, [], me.user.id), true);
  for (const mutate of [(v: any) => v.archive.summary = 'Other summary', (v: any) => v.archive.fingerprint = 'b'.repeat(32),
    (v: any) => v.archive.project_version++, (v: any) => v.archive.project_id = id(99), (v: any) => v.archive.actor_id = id(99),
    (v: any) => v.archive.current_basis_state = 'changed', (v: any) => v.archive.groups.pop(), (v: any) => v.project.scope = 'Different scope']) {
    const bad = copy(value); mutate(bad); assert.equal(archiveReceipt(bad, b, body, [], me.user.id), false);
  }
});

test('fixed attachment receipts preserve ordered IDs, large byte text, original file identity and complete metadata', () => {
  const b = basis(), { body, value } = receipt(b, [file]);
  assert.equal(archiveReceipt(value, b, body, [file], me.user.id), true);
  assert.equal(archiveReceipt(value, b, body, [], me.user.id), false);
  assert.equal(value.archive.attachments[0].size_bytes, file.size_bytes);
  for (const change of [{ sha256: 'c'.repeat(64) }, { file_id: id(99) }, { size_bytes: '1' }, { seq: 1 }, { name: 'Different original' }]) {
    const bad = copy(value); Object.assign(bad.archive.attachments[0], change); assert.equal(archiveReceipt(bad, b, body, [file], me.user.id), false);
  }
});

test('historical reads reject mixed projects, missing records and restricted payload leakage instead of showing an empty success', () => {
  const { value } = receipt(), complete = { visibility: 'complete' as const, items: [value.archive], next_cursor: null };
  assert.equal(currentArchives(complete, project.id).items.length, 1);
  assert.equal(currentArchives({ visibility: 'restricted', items: [], next_cursor: null }, project.id).visibility, 'restricted');
  for (const bad of [{ ...complete, visibility: 'restricted' }, { visibility: 'restricted', items: [], next_cursor: 'hidden' },
    { ...complete, items: [value.archive, value.archive] }, { ...complete, items: [{ ...value.archive, project_id: id(99) }] }]) assert.throws(() => currentArchives(bad, project.id));
});

test('a committed archive needs actual current project and history proof; hidden history or a concurrent reopen blocks success', () => {
  const b = basis(), { body, value } = receipt(b), current = { ...basis(), project: value.project, fingerprint: 'c'.repeat(32), can_archive: false };
  const history = { visibility: 'complete' as const, items: [value.archive], next_cursor: null };
  assert.equal(archiveProof(current, history, value, b, body, [], me.user.id), true);
  assert.equal(archiveProof(current, { ...history, items: [] }, value, b, body, [], me.user.id), false);
  assert.equal(archiveProof(current, { visibility: 'restricted', items: [], next_cursor: null }, value, b, body, [], me.user.id), false);
  assert.equal(archiveProof({ ...current, project: { ...current.project, version: 9, state: 'active' } }, history, value, b, body, [], me.user.id), false);
  assert.equal(archiveProof(current, { ...history, items: [{ ...value.archive, current_basis_state: 'changed' }] }, value, b, body, [], me.user.id), false);
  const changed = copy(current); changed.groups[0] = { kind: 'tasks', visibility: 'complete', count: 1, open_count: 0, items: [{ kind: 'tasks', id: id(40), version: 10, title: 'Changed actual task', state: 'done', blocking: false }] };
  assert.equal(archiveProof(changed, history, value, b, body, [], me.user.id), false);
});

test('an unfinished source cannot be represented as ready by an invented capability or incomplete category', () => {
  const b = basis(); b.groups[0] = { kind: 'tasks', visibility: 'complete', count: 1, open_count: 1,
    items: [{ kind: 'tasks', id: id(40), version: 9, title: 'Actual unfinished task', state: 'in_review', blocking: true }] };
  assert.throws(() => currentCloseout(b, project.id)); b.can_archive = false;
  assert.equal(currentCloseout(b, project.id).groups[0].open_count, 1);
  assert.throws(() => archivePayload(b, members, me, notes, [], true, true, true));
});
