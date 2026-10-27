import { expect, it } from 'vitest';
import { makeTaskDependency, makeMutation, mutationReceipt, mutationTargetType, type MutationView } from '@copypaste/contracts/mutation';
import type { Schemas } from './api';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const task = { id: id(1), version: 4, project_id: id(3), state: 'todo' as const }, pred = { id: id(2), version: 7, project_id: id(3), state: 'todo' as const };
const input = (action = 'add') => makeTaskDependency(task, pred, action, '员工确认这一条依赖', action === 'remove' ? [pred.id] : []);
function view(action = 'add'): MutationView {
  const before = { ...task, title: '发布交付', body: '保留说明', priority: 'normal', assignee_id: null, parent_id: null, milestone_id: null, review_required: true, start_at: null, due_at: null,
    predecessor_id: pred.id, predecessor_version: pred.version, predecessor_title: '核对资料', predecessor_state: 'todo', predecessor_sha256: 'a'.repeat(64), dependency_present: action === 'remove' };
  return { run_id: id(4), tool: 'update_task_dependency', resource_type: 'task', resource_id: task.id, base_version: task.version, intent_sha256: 'b'.repeat(64), arguments: input(action).arguments, before,
    status: 'committed', operation_id: id(5), result_version: 5, result: { ...task, title: before.title, body: before.body, priority: before.priority, assignee_id: null, parent_id: null, milestone_id: null, review_required: true, version: 5, predecessor_id: pred.id, predecessor_version: pred.version, dependency_action: action, dependency_present: action === 'add' } };
}
const ack = (m = view(), action = 'add') => mutationReceipt({ id: id(4), mutation: m } as Schemas['Run'], input(action));

it('fixes both versions, one edge and the exact action without generic patch authority', () => {
  expect(input().arguments).toEqual({ id: task.id, expected_version: 4, predecessor_id: pred.id, expected_predecessor_version: 7, action: 'add', reason: '员工确认这一条依赖' });
  expect(mutationTargetType('update_task_dependency')).toBe('task');
  expect(() => makeMutation('update_task_dependency', task, 'action', 'add', false, 'Reviewed')).toThrow();
});
it('requires different same-project tasks with real IDs and positive exact versions', () => {
  for (const invalid of [task, { ...pred, project_id: id(9) }, { ...pred, id: 'guess' }, { ...pred, version: 0 }, { ...pred, version: 1.5 }]) expect(() => makeTaskDependency(task, invalid, 'add', 'Reviewed', [])).toThrow();
});
it('rejects inferred actions, blank reasons, duplicate additions and absent removals', () => {
  for (const [action, reason, edges] of [['', 'Reviewed', []], ['replace', 'Reviewed', []], ['add', ' ', []], ['add', 'Reviewed', [pred.id]], ['remove', 'Reviewed', []]] as const) expect(() => makeTaskDependency(task, pred, action, reason, edges)).toThrow();
});
it('retains task state blockers while allowing an already completed prerequisite', () => {
  for (const state of ['done', 'cancelled', 'in_progress', 'in_review'] as const) expect(() => makeTaskDependency({ ...task, state }, pred, 'add', 'Reviewed', [])).toThrow();
  expect(makeTaskDependency({ ...task, state: 'in_progress' }, { ...pred, state: 'done' }, 'add', 'Reviewed', [])).toBeDefined();
});
it('acknowledges both actual add and actual remove receipts', () => { expect(ack()).toBe(true); expect(ack(view('remove'), 'remove')).toBe(true); });
it('preserves task dates at full microsecond precision across native timestamp formats', () => {
  const m = view(); m.before.due_at = '2027-03-05T18:00:00.123456+08:00'; m.result!.due_at = '2027-03-05T10:00:00.123456Z'; expect(ack(m)).toBe(true);
  m.result!.due_at = '2027-03-05T10:00:00.123457Z'; expect(ack(m)).toBe(false);
});
it('rejects forged edges, predecessor versions, actions, projects and unauthorized metadata', () => {
  for (const patch of [{ dependency_present: false }, { predecessor_id: id(9) }, { predecessor_version: 8 }, { dependency_action: 'remove' }, { project_id: id(9) }, { body: 'Changed without authority' }, { review_required: false }, { version: 6 }]) { const m = view(); m.result = { ...m.result, ...patch }; expect(ack(m)).toBe(false); }
});
it('keeps authorization distinct from commit and requires its original predecessor basis', () => {
  const m = view(); m.status = 'authorized'; m.result = null; m.result_version = null; m.operation_id = null; expect(ack(m)).toBe(true);
  for (const patch of [{ predecessor_id: id(9) }, { predecessor_version: 8 }, { predecessor_sha256: 'bad' }, { dependency_present: true }]) expect(ack({ ...m, before: { ...m.before, ...patch } })).toBe(false);
  expect(ack({ ...view(), operation_id: null })).toBe(false);
});
