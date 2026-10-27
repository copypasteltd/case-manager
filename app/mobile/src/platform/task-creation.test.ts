import assert from 'node:assert/strict';
import { test } from 'node:test';
import { makeTaskCreation, makeMutation, mutationReceipt, type TaskCreationValues, type MutationView } from '../../../packages/contracts/src/mutation';
import type { components } from '../../../packages/contracts/src/api';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const project = { id: id(1), version: 9, state: 'on_hold' };
const values = (): TaskCreationValues => ({ title: '移动端完整任务', body: '', assignee_id: null, parent_id: null, milestone_id: null, start_at: null, due_at: null, priority: 'normal', review_required: false });
test('mobile sends the exact complete nullable task and explicit false review choice', () => {
  const v = values(), a = makeTaskCreation(project, v, '员工明确核对'); assert.deepEqual(a.arguments, { id: project.id, expected_version: 9, reason: '员工明确核对', ...v });
  assert.throws(() => makeMutation('create_task', project, 'title', 'Only a title', false, 'Reviewed'));
});
test('mobile rejects missing review choices, guessed references and hidden dependencies', () => {
  for (const patch of [{ review_required: undefined }, { assignee_id: 'someone' }, { dependency_ids: [id(2)] }]) assert.throws(() => makeTaskCreation(project, { ...values(), ...patch } as TaskCreationValues, 'Reviewed'));
});
test('mobile compares zoned microsecond instants before creating a subtask', () => {
  const v = { ...values(), parent_id: id(2), start_at: '2027-03-05T18:00:00.000002+08:00', due_at: '2027-03-05T10:00:00.000001Z' };
  assert.throws(() => makeTaskCreation(project, v, 'Reviewed'));
  assert.equal(makeTaskCreation(project, { ...v, due_at: '2027-03-05T10:00:00.000002Z' }, 'Reviewed').arguments.parent_id, id(2));
});
test('mobile acknowledges only an actual committed task with matching review and assignment', () => {
  const a = makeTaskCreation(project, values(), 'Reviewed');
  const m: MutationView = { run_id: id(3), tool: 'create_task', resource_type: 'project', resource_id: project.id, base_version: 9, intent_sha256: 'a'.repeat(64), arguments: a.arguments, before: project, status: 'committed', operation_id: id(4), result_version: 1, result: { ...values(), id: id(5), version: 1, project_id: project.id, creator_id: id(6), state: 'todo' } };
  const run = { id: id(3), actor_id: id(6), mutation: m } as components['schemas']['Run']; assert.equal(mutationReceipt(run, a), true);
  m.result = { ...m.result, review_required: true }; assert.equal(mutationReceipt(run, a), false);
});
