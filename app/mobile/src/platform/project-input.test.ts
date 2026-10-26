import test from 'node:test';
import assert from 'node:assert/strict';
import type { Schemas } from './session';
import { currentProjectInput, copyProjectInput, projectPayload, projectReceipt, memberPayload, ownerPayload, projectVersionReceipt } from './project-input';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const me = { user: { id: id(1) }, authorization_version: 1 } as Schemas['Me'];
const base = { id: id(10), version: 7, name: 'Current project', scope: 'Approved scope', type: 'general', owner_id: id(1), classification: 'project_restricted', project_no: 'PRJ-000001', state: 'draft', initialization_state: 'complete' } as Schemas['Project'];
const input = { ...currentProjectInput(), name: 'New project', scope: 'Human scope', start: '2028-02-29', due: '2028-03-01' };
const template = { id: id(20), version: 4, revision: 2, state: 'published', content_hash: 'a'.repeat(64) } as Schemas['ProjectTemplate'];

test('project creation fixes the logged-in owner, exact template revision identity and unshifted calendar dates', () => {
  assert.deepEqual(projectPayload(input, me, undefined, template), { name: 'New project', type: 'general', scope: 'Human scope', owner_id: id(1), classification: 'project_restricted', start_date: '2028-02-29', due_date: '2028-03-01', template_id: id(20) });
  assert.throws(() => projectPayload(input, me, undefined, { ...template, state: 'archived' }));
  assert.throws(() => projectPayload({ ...input, classification: 'confidential' }, me));
  assert.throws(() => projectPayload({ ...input, name: '\u0085\u3000' }, me));
});
test('invalid or inverted calendar dates never become normalized approved business dates', () => {
  for (const start of ['2026-02-29', '2028-02-30', '2028-13-01', '2028-2-01', '0000-01-01', '2028-01-01Z']) assert.throws(() => projectPayload({ ...input, start }, me), start);
  assert.throws(() => projectPayload({ ...input, start: '2028-03-02' }, me));
  assert.deepEqual(projectPayload({ ...input, start: '', due: '' }, me), { name: 'New project', type: 'general', scope: 'Human scope', owner_id: id(1), classification: 'project_restricted' });
});
test('draft edits explicitly clear dates, while active scope and dates require their original change workflow', () => {
  const draft = currentProjectInput(base);
  assert.deepEqual(projectPayload(draft, me, base), { expected_version: 7, name: base.name, type: 'general', scope: base.scope, start_date: null, due_date: null });
  const active = { ...base, state: 'active' as const };
  assert.deepEqual(projectPayload(draft, me, active), { expected_version: 7, name: base.name, type: 'general' });
  assert.throws(() => projectPayload({ ...draft, scope: 'Unapproved' }, me, active));
  assert.throws(() => projectPayload({ ...draft, due: '2028-03-01' }, me, active));
  for (const state of ['closing', 'archived', 'cancelled'] as const) assert.throws(() => projectPayload(draft, me, { ...base, state }));
});
test('creation receipts must prove the exact new project and template revision without fabricating initialization', () => {
  const receipt = { ...base, id: id(30), version: 1, name: input.name, scope: input.scope, start_date: input.start, due_date: input.due, template_id: template.id, template_version: 2 };
  assert.equal(projectReceipt(receipt, input, me, undefined, template), true);
  for (const patch of [{ version: 8 }, { owner_id: id(3) }, { state: 'active' }, { template_version: 4 }, { due_date: '2028-03-02' }, { initialization_state: 'unknown' }, { id: 'fake' }]) assert.equal(projectReceipt({ ...receipt, ...patch }, input, me, undefined, template), false);
  assert.equal(projectReceipt({ ...receipt, initialization_state: 'partial_failed' }, input, me, undefined, template), true);
});
test('Unicode whitespace normalization and existing resource versions remain bound to the actual project receipt', () => {
  const edit = { ...currentProjectInput(base), name: '\u0085\u3000Updated\t' };
  assert.deepEqual(projectPayload(edit, me, base), { expected_version: 7, name: 'Updated', type: 'general', scope: base.scope, start_date: null, due_date: null });
  assert.equal(projectReceipt({ ...base, version: 8, name: 'Updated' }, edit, me, base), true);
  assert.equal(projectReceipt({ ...base, id: id(12), version: 8, name: 'Updated' }, edit, me, base), false);
});
test('membership changes preserve the parent version, explicit role and reason and reject self or owner changes', () => {
  const members = { items: [{ id: id(2) }] } as Schemas['ProjectMembers'];
  assert.deepEqual(memberPayload(base, members, me, id(2), 'viewer', ' Read only '), { expected_version: 7, user_id: id(2), action: 'set', project_role: 'viewer', reason: 'Read only' });
  assert.deepEqual(memberPayload(base, members, me, id(2), 'remove', 'Closed work'), { expected_version: 7, user_id: id(2), action: 'remove', reason: 'Closed work' });
  for (const user of [id(1), 'fake']) assert.throws(() => memberPayload(base, members, me, user, 'member', 'Reason'));
  assert.throws(() => memberPayload(base, members, me, id(3), 'remove', 'Reason'));
  assert.throws(() => memberPayload(base, members, me, id(2), 'owner', 'Reason'));
  assert.throws(() => memberPayload({ ...base, state: 'archived' }, members, me, id(2), 'viewer', 'Reason'));
});
test('owner handover uses its distinct command and a receipt for the exact new owner and next project version', () => {
  assert.deepEqual(ownerPayload(base, id(2)), { expected_version: 7, owner_id: id(2) });
  assert.throws(() => ownerPayload(base, base.owner_id));
  assert.equal(projectVersionReceipt({ ...base, version: 8, owner_id: id(2) }, base, id(2)), true);
  assert.equal(projectVersionReceipt({ ...base, version: 8 }, base, id(2)), false);
  assert.equal(projectVersionReceipt({ ...base, version: 7, owner_id: id(2) }, base, id(2)), false);
});
test('retained project input strips template evidence, arbitrary owner and confirmation flags and bounds text', () => {
  assert.deepEqual(copyProjectInput({ ...input, template, owner_id: id(3), confirmed: true, access_token: 'never retained' }), input);
  assert.equal(copyProjectInput({ ...input, scope: 'x'.repeat(20001) }), undefined);
  assert.equal(copyProjectInput({ ...input, due: null }), undefined);
});
