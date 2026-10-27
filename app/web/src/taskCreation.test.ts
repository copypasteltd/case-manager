import { expect, it } from 'vitest';
import { makeTaskCreation, makeMutation, mutationChanges, mutationReceipt, mutationTargetType, type TaskCreationValues, type MutationView } from '@copypaste/contracts/mutation';
import type { Schemas } from './api';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const project = { id: id(1), version: 8, state: 'active' };
const values = (): TaskCreationValues => ({ title: '\u0085 新任务\u3000', body: '完整核对资料', assignee_id: id(2), parent_id: id(3), milestone_id: id(4), start_at: '2027-03-05T10:00:00.123456+08:00', due_at: '2027-03-05T18:00:00.123457+08:00', priority: 'high', review_required: true });
const input = () => makeTaskCreation(project, values(), '准确创建本次任务');
function receipt(): MutationView {
  const a = input();
  return { run_id: id(5), tool: 'create_task', resource_type: 'project', resource_id: project.id, base_version: project.version, intent_sha256: 'a'.repeat(64), arguments: a.arguments, before: project, status: 'committed', operation_id: id(6), result_version: 1,
    result: { ...values(), id: id(7), version: 1, project_id: project.id, creator_id: id(8), title: '新任务', state: 'todo', start_at: '2027-03-05T02:00:00.123456Z', due_at: '2027-03-05T10:00:00.123457Z' } };
}
function ack(m = receipt()) { return mutationReceipt({ id: id(5), actor_id: id(8), mutation: m } as Schemas['Run'], input()); }

it('binds nine complete fields to the parent and displays explicit human review', () => {
  expect(input().arguments).toEqual({ id: project.id, expected_version: 8, reason: '准确创建本次任务', ...values() });
  expect(mutationTargetType('create_task')).toBe('project');
  expect(mutationChanges(input()).find(f => f.field === 'review_required')?.value).toBe('需要人工验收');
  expect(() => makeMutation('create_task', project, 'title', 'Partial authority', false, 'Reviewed')).toThrow();
});
it('requires all fields and rejects hidden defaults or extra authority', () => {
  for (const key of Object.keys(values())) { const v = values() as unknown as Record<string, unknown>; delete v[key]; expect(() => makeTaskCreation(project, v as unknown as TaskCreationValues, 'Reviewed')).toThrow(); }
  for (const patch of [{ review_required: null }, { review_required: 'false' }, { state: 'done' }, { dependency_ids: [id(3)] }, { assignee_id: '' }, { title: '\ud800' }, { body: '中'.repeat(20001) }]) expect(() => makeTaskCreation(project, { ...values(), ...patch } as TaskCreationValues, 'Reviewed')).toThrow();
  for (const state of ['closing', 'archived', 'cancelled']) expect(() => makeTaskCreation({ ...project, state }, values(), 'Reviewed')).toThrow();
});
it('preserves null and false choices instead of assigning or requiring review', () => {
  const v = { ...values(), assignee_id: null, parent_id: null, milestone_id: null, start_at: null, due_at: null, review_required: false, body: '' };
  expect(makeTaskCreation(project, v, 'Reviewed').arguments).toMatchObject(v);
  expect(mutationChanges(makeTaskCreation(project, v, 'Reviewed')).find(f => f.field === 'assignee_id')?.value).toBe('明确不设置');
});
it('rejects impossible dates, excess precision and microsecond inversions', () => {
  for (const due_at of ['2027-02-30T10:00:00Z', '2027-03-05T10:00:00', '2027-03-05T10:00:00.1234567+08:00', '2027-03-05T10:00:00.123455+08:00']) expect(() => makeTaskCreation(project, { ...values(), due_at }, 'Reviewed')).toThrow();
  expect(makeTaskCreation(project, { ...values(), due_at: '2027-03-05T02:00:00.123456Z' }, 'Reviewed')).toBeDefined();
});
it('accepts only genuine new todo task receipts with equal actual instants', () => {
  expect(ack()).toBe(true);
  for (const patch of [{ id: project.id }, { version: 2 }, { state: 'done' }, { project_id: id(9) }, { creator_id: id(9) }, { review_required: false }, { parent_id: null }, { milestone_id: null }, { assignee_id: id(9) }, { body: 'Altered' }, { due_at: '2027-03-05T10:00:00.123458Z' }]) { const m = receipt(); m.result = { ...m.result, ...patch }; expect(ack(m)).toBe(false); }
});
it('rejects missing operation, altered authority and preexisting child versions', () => {
  for (const patch of [{ operation_id: null }, { result_version: 2 }, { run_id: id(9) }, { resource_type: 'task' }, { status: 'authorized' }]) expect(ack({ ...receipt(), ...patch } as MutationView)).toBe(false);
  const m = receipt(); m.arguments = { ...m.arguments, review_required: false }; expect(ack(m)).toBe(false);
});
it('applies Unicode limits and a byte bound to the complete payload', () => {
  expect(makeTaskCreation(project, { ...values(), title: '😀'.repeat(200), body: '中'.repeat(20000) }, 'Reviewed')).toBeDefined();
  expect(() => makeTaskCreation(project, { ...values(), title: '😀'.repeat(201) }, 'Reviewed')).toThrow();
  expect(() => makeTaskCreation(project, { ...values(), body: '😀'.repeat(20000) }, 'Reviewed')).toThrow(/64KiB/);
});
