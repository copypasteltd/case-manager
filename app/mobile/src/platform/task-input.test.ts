import test from 'node:test';
import assert from 'node:assert/strict';
import type { Schemas } from './session';
import { copyTaskInput, currentTaskInput, dependencyPayload, dependencyProof, dependencyTaskReceipt, taskPayload, taskReceipt, taskWritable } from './task-input';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const me = { user: { id: id(2) }, actions: ['task.create', 'task.update', 'task.assign'] } as Schemas['Me'];
const project = { id: id(1), version: 6, state: 'active', owner_id: me.user.id } as Schemas['Project'];
const members = { items: [{ id: me.user.id, status: 'active' }, { id: id(3), status: 'active' }, { id: id(4), status: 'disabled' }], memberships: [{ user_id: me.user.id, project_role: 'manager' }] } as Schemas['ProjectMembers'];
const before = { id: id(10), project_id: project.id, version: 7, state: 'todo', title: 'Original', body: 'Original body', assignee_id: id(3), parent_id: null, milestone_id: null, start_at: '2028-02-29T01:00:00.123456Z', due_at: '2028-02-29T10:00:00.123457Z', priority: 'normal', review_required: true } as Schemas['Task'];
const parent = { ...before, id: id(11) }, milestone = { id: id(20), project_id: project.id, state: 'open' } as Schemas['Milestone'];
const input = { ...currentTaskInput(), title: '\u0085 New task \u3000', body: 'Actual acceptance criteria', assignee: id(3), parent: parent.id, milestone: milestone.id, start: '2028-02-29 09:00:00.123456', due: '2028-02-29 09:00:00.123457', priority: 'high', review: 'required' };
const refs = { parent, milestone };

test('creation retains full content, nullable fields and exact Hong Kong instants without a fake parent version', () => {
  const body = taskPayload(input, project, members, me, undefined, refs) as Schemas['TaskCreate'];
  assert.equal(body.title, 'New task'); assert.equal(body.body, input.body); assert.equal(body.parent_id, parent.id); assert.equal(body.milestone_id, milestone.id);
  assert.equal(body.start_at, '2028-02-29T01:00:00.123456Z'); assert.equal(body.due_at, '2028-02-29T01:00:00.123457Z'); assert.equal(body.review_required, true);
  assert.equal('expected_version' in body, false);
  const empty = taskPayload({ ...currentTaskInput(), title: 'Unscheduled' }, project, members, me) as Schemas['TaskCreate'];
  assert.equal(empty.assignee_id, null); assert.equal(empty.parent_id, null); assert.equal(empty.milestone_id, null); assert.equal(empty.start_at, null); assert.equal(empty.due_at, null);
});

test('strict date validation rejects impossible dates and reversed times at microsecond precision', () => {
  for (const start of ['2026-02-29 09:00', '2028-04-31 09:00', '2028-02-29 24:00', '2028-02-29T09:00:00+25:00', '2028-02-29 09:00:00.1234567']) assert.throws(() => taskPayload({ ...input, start }, project, members, me, undefined, refs));
  assert.throws(() => taskPayload({ ...input, due: '2028-02-29 09:00:00.123455' }, project, members, me, undefined, refs), /截止时间/);
  assert.equal(currentTaskInput(before).start, '2028-02-29 09:00:00.123456');
});

test('assignment follows original create and update permissions, including explicit clearing', () => {
  const selfOnly = { ...me, actions: ['task.create', 'task.update'] };
  assert.throws(() => taskPayload(input, project, members, selfOnly, undefined, refs), /分配权限/);
  assert.equal((taskPayload({ ...input, assignee: me.user.id }, project, members, selfOnly, undefined, refs) as Schemas['TaskCreate']).assignee_id, me.user.id);
  const untouched = taskPayload(currentTaskInput(before), project, members, selfOnly, before);
  assert.equal('assignee_id' in untouched, false);
  assert.throws(() => taskPayload({ ...currentTaskInput(before), assignee: '' }, project, members, selfOnly, before), /分配权限/);
  assert.equal((taskPayload({ ...currentTaskInput(before), assignee: '' }, project, members, me, before) as Schemas['TaskPatch']).assignee_id, null);
  for (const assignee of [id(4), id(99)]) assert.throws(() => taskPayload({ ...input, assignee }, project, members, me, undefined, refs), /有效成员/);
});

test('parents and milestones require actual same-project readable candidates while unchanged references remain exact', () => {
  for (const parent of [{ ...refs.parent, project_id: id(99) }, { ...refs.parent, state: 'done' as const }, { ...refs.parent, id: id(99) }]) assert.throws(() => taskPayload(input, project, members, me, undefined, { ...refs, parent }));
  for (const milestone of [{ ...refs.milestone, project_id: id(99) }, { ...refs.milestone, state: 'achieved' as const }]) assert.throws(() => taskPayload(input, project, members, me, undefined, { ...refs, milestone }));
  assert.throws(() => taskPayload({ ...currentTaskInput(before), parent: before.id }, project, members, me, before));
  const linked = { ...before, parent_id: parent.id, milestone_id: milestone.id };
  assert.equal((taskPayload(currentTaskInput(linked), project, members, me, linked) as Schemas['TaskPatch']).parent_id, parent.id);
  const cleared = taskPayload({ ...currentTaskInput(linked), parent: '', milestone: '', start: '', due: '' }, project, members, me, linked) as Schemas['TaskPatch'];
  for (const key of ['parent_id', 'milestone_id', 'start_at', 'due_at'] as const) assert.equal(cleared[key], null);
});

test('viewer roles, stopped projects, completed tasks and wrong-project deep links remain read only', () => {
  const viewer = { ...members, memberships: [{ ...members.memberships![0], project_role: 'viewer' as const }] };
  assert.equal(taskWritable(project, viewer, me), false); assert.throws(() => taskPayload(input, project, viewer, me, undefined, refs));
  for (const state of ['closing', 'archived', 'cancelled'] as const) assert.throws(() => taskPayload(input, { ...project, state }, members, me, undefined, refs));
  for (const state of ['done', 'cancelled'] as const) assert.throws(() => taskPayload(currentTaskInput(before), project, members, me, { ...before, state }));
  assert.throws(() => taskPayload(currentTaskInput(before), project, members, me, { ...before, project_id: id(99) }));
});

test('successful task receipt must match every intended field, identity, version and microsecond', () => {
  const body = taskPayload(input, project, members, me, undefined, refs);
  const receipt = { ...body, id: id(30), project_id: project.id, version: 1, state: 'todo' } as Schemas['Task'];
  assert.equal(taskReceipt(receipt, input, project), true);
  for (const change of [{ version: 2 }, { id: 'invalid' }, { project_id: id(99) }, { title: 'Wrong' }, { body: 'Truncated' }, { state: 'done' }, { assignee_id: null }, { parent_id: null }, { milestone_id: null }, { priority: 'low' }, { review_required: false }, { start_at: '2028-02-29T01:00:00.123455Z' }, { due_at: null }, { due_at: 'not-a-time' }]) assert.equal(taskReceipt({ ...receipt, ...change }, input, project), false);
  const editing = currentTaskInput(before), updated = { ...before, version: 8 };
  assert.equal(taskReceipt(updated, editing, project, before), true); assert.equal(taskReceipt({ ...updated, id: id(99) }, editing, project, before), false);
});

test('dependency commands retain exact successor version, same project and lifecycle constraints', () => {
  assert.deepEqual(dependencyPayload(before, parent, project, 'add'), { expected_version: 7, predecessor_id: parent.id, action: 'add' });
  assert.throws(() => dependencyPayload(before, before, project, 'add'));
  assert.throws(() => dependencyPayload(before, { ...parent, project_id: id(99) }, project, 'add'));
  assert.throws(() => dependencyPayload(before, parent, project, 'replace'));
  const running = { ...before, state: 'in_progress' as const };
  assert.throws(() => dependencyPayload(running, parent, project, 'add'), /未完成前置任务/);
  assert.equal(dependencyPayload(running, { ...parent, state: 'done' }, project, 'add').action, 'add');
  assert.equal(dependencyPayload(running, parent, project, 'remove').action, 'remove');
});

test('dependency success requires a complete task receipt and actual edge proof at the same task revision', () => {
  const receipt = { ...before, version: 8 }, edges = { items: [{ predecessor_id: parent.id, title: parent.title, state: parent.state }] };
  assert.equal(dependencyTaskReceipt(receipt, before), true);
  assert.equal(dependencyTaskReceipt({ ...receipt, body: 'Changed' }, before), false);
  assert.equal(dependencyTaskReceipt({ ...receipt, start_at: '2028-02-29T01:00:00.123455Z' }, before), false);
  assert.equal(dependencyProof(edges, receipt, receipt, parent, 'add'), true);
  assert.equal(dependencyProof({ items: [] }, receipt, receipt, parent, 'add'), false);
  assert.equal(dependencyProof({ items: [] }, receipt, receipt, parent, 'remove'), true);
  assert.equal(dependencyProof(edges, { ...receipt, version: 9 }, receipt, parent, 'add'), false);
  assert.equal(dependencyProof({ items: [...edges.items, ...edges.items] }, receipt, receipt, parent, 'add'), false);
  assert.equal(dependencyProof({ items: [{ predecessor_id: parent.id }] }, receipt, receipt, parent, 'add'), false);
});

test('task drafts copy only bounded text fields and reject oversized or mistyped inputs', () => {
  const copied = copyTaskInput({ ...input, token: 'private', complete_response: before });
  assert.deepEqual(copied, input); assert.notEqual(copied, input);
  for (const change of [{ title: 'x'.repeat(201) }, { body: 'x'.repeat(20001) }, { assignee: id(2) + 'x' }, { review: true }, { start: [] }]) assert.equal(copyTaskInput({ ...input, ...change }), undefined);
});
