import test from 'node:test';
import assert from 'node:assert/strict';
import { makeConcernMutation, mutationReceipt, mutationTargetType, type ConcernValues, type MutationView } from '../../../packages/contracts/src/mutation';
import type { Schemas } from './session';

const id = (n: number) => '20000000-0000-4000-8000-' + String(n).padStart(12, '0');
const project = { id: id(1), version: 6, state: 'active' }, risk = { id: id(2), version: 3, state: 'open' };
const values: ConcernValues = { title: '移动风险 😀', description: '实际完整资料', owner_id: id(3), severity: 'high', due_at: null, probability: null, mitigation: '复核原始依据' };

test('Taro concern payload serialization preserves explicit clears, all metadata and an omitted state', () => {
  const input = makeConcernMutation('update_risk', risk, values, '核对准确风险');
  assert.deepEqual(JSON.parse(JSON.stringify(input)).arguments, { id: risk.id, expected_version: 3, reason: '核对准确风险', ...values });
  assert.equal(Object.hasOwn(input.arguments, 'state'), false);
  assert.equal(mutationTargetType('create_risk'), 'project'); assert.equal(mutationTargetType('update_issue'), 'issue');
});
test('Taro requires every field, valid Unicode and explicit writable state before preparing a concern', () => {
  for (const key of Object.keys(values)) { const copy = { ...values }; delete (copy as unknown as Record<string,unknown>)[key]; assert.throws(() => makeConcernMutation('create_risk', project, copy, '完整创建')); }
  for (const change of [{ title: '\udfff' }, { probability: 'certain' }, { due_at: '2027-02-30T18:00:00Z' }, { state: 'closed' }]) assert.throws(() => makeConcernMutation('create_risk', project, { ...values, ...change }, '完整创建'));
  assert.throws(() => makeConcernMutation('create_risk', { ...project, state: 'archived' }, values, '只读项目'));
});
test('Taro confirms the exact new child, parent version and sender rather than a parent update', () => {
  const input = makeConcernMutation('create_risk', project, values, '明确创建');
  const m: MutationView = { run_id: id(4), tool: input.tool, resource_type: 'project', resource_id: project.id, base_version: 6, intent_sha256: 'a'.repeat(64), arguments: input.arguments, before: { id: project.id, version: 6 }, status: 'committed', operation_id: id(5), result_version: 1,
    result: { ...values, id: id(6), version: 1, project_id: project.id, creator_id: id(7), state: 'open' } };
  const run = { id: id(4), actor_id: id(7), mutation: m } as Schemas['Run'];
  assert.equal(mutationReceipt(run, input), true);
  for (const change of [{ id: project.id }, { version: 7 }, { creator_id: id(8) }, { state: 'closed' }, { description: '替换正文' }]) assert.equal(mutationReceipt({ ...run, mutation: { ...m, result: { ...m.result, ...change } } }, input), false);
});
test('Taro risk realization cannot use a missing, reused or malformed linked issue', () => {
  const input = makeConcernMutation('update_risk', risk, { ...values, state: 'realized' }, '发生并转为问题');
  const m: MutationView = { run_id: id(4), tool: input.tool, resource_type: 'risk', resource_id: risk.id, base_version: 3, intent_sha256: 'b'.repeat(64), arguments: input.arguments, before: { id: risk.id, version: 3, project_id: project.id, creator_id: id(7), state: 'open', issue_id: null }, status: 'committed', operation_id: id(5), result_version: 4,
    result: { ...values, id: risk.id, version: 4, project_id: project.id, creator_id: id(7), state: 'realized' } };
  const run = { id: id(4), actor_id: id(7), mutation: m } as Schemas['Run'];
  assert.equal(mutationReceipt(run, input), false);
  assert.equal(mutationReceipt({ ...run, mutation: { ...m, result: { ...m.result, issue_id: id(9) } } }, input), true);
  for (const issue_id of [risk.id, project.id, 'bad-id']) assert.equal(mutationReceipt({ ...run, mutation: { ...m, result: { ...m.result, issue_id } } }, input), false);
});
test('Taro issue completion requires resolution and follows the original state transitions', () => {
  const issue = { ...risk, state: 'open' }, v: ConcernValues = { title: '移动问题', description: '', owner_id: id(3), severity: 'medium', due_at: null, resolution: '' };
  assert.throws(() => makeConcernMutation('update_issue', issue, { ...v, state: 'resolved' }, '解决'));
  assert.throws(() => makeConcernMutation('update_issue', issue, { ...v, resolution: '实际依据', state: 'closed' }, '跳过解决'));
  assert.equal(makeConcernMutation('update_issue', { ...issue, state: 'resolved' }, { ...v, resolution: '实际依据', state: 'closed' }, '明确关闭').arguments.state, 'closed');
});
