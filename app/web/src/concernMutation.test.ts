import { expect, it } from 'vitest';
import { makeConcernMutation, makeMutation, mutationReceipt, mutationTargetType, type ConcernValues, type Mutation, type MutationView } from '@copypaste/contracts/mutation';
import type { Schemas } from './api';

const id = (n: number) => '10000000-0000-4000-8000-' + String(n).padStart(12, '0');
const parent = { id: id(1), version: 7, state: 'active' }, record = { id: id(2), version: 3, state: 'open' };
const values = (): ConcernValues => ({ title: '\u0085 风险标题\u3000', description: 'Actual evidence 中文', owner_id: id(3), severity: 'high', due_at: '2027-03-05T18:00:00.123456+08:00', probability: null, mitigation: '' });
function view(input: Mutation, creating = input.tool.startsWith('create_')): MutationView {
  return { run_id: id(8), tool: input.tool, resource_type: mutationTargetType(input.tool), resource_id: input.arguments.id, base_version: input.arguments.expected_version, intent_sha256: 'a'.repeat(64), arguments: input.arguments,
    before: { id: input.arguments.id, version: input.arguments.expected_version, project_id: parent.id, creator_id: id(9), state: 'open', issue_id: null }, status: 'committed', operation_id: id(7), result_version: creating ? 1 : 4,
    result: { ...input.arguments, id: creating ? id(4) : record.id, version: creating ? 1 : 4, project_id: parent.id, creator_id: creating ? id(6) : id(9), state: input.arguments.state ?? 'open', title: '风险标题', due_at: '2027-03-05T10:00:00.123456Z' } };
}
function ack(input: Mutation, m = view(input)): boolean { return mutationReceipt({ id: id(8), actor_id: id(6), mutation: m } as Schemas['Run'], input); }

it('fixes complete metadata, explicit empty fields and distinct parent/child targets', () => {
  const v = values(), input = makeConcernMutation('create_risk', parent, v, 'Reviewed creation');
  expect(input.arguments).toEqual({ id: parent.id, expected_version: 7, reason: 'Reviewed creation', ...v });
  expect(input.arguments).not.toHaveProperty('state');
  expect(mutationTargetType('create_risk')).toBe('project'); expect(mutationTargetType('update_risk')).toBe('risk');
  expect(() => makeMutation('update_risk', record, 'title', 'Partial', false, 'Reason')).toThrow();
});
it('rejects absent metadata, cross-kind fields, invalid Unicode and oversized UTF8 authority', () => {
  for (const field of ['title', 'description', 'owner_id', 'severity', 'due_at', 'probability', 'mitigation']) { const v = values(); delete (v as unknown as Record<string,unknown>)[field]; expect(() => makeConcernMutation('create_risk', parent, v, 'Reason')).toThrow(); }
  for (const change of [{ title: '\ud800' }, { severity: 'urgent' }, { probability: 'certain' }, { resolution: 'Wrong kind' }, { state: 'closed' }, { description: '中'.repeat(20000), mitigation: '中'.repeat(20000) }]) expect(() => makeConcernMutation('create_risk', parent, { ...values(), ...change }, 'Reason')).toThrow();
});
it('rejects impossible, missing-zone and sub-microsecond times and preserves accurate microseconds', () => {
  for (const due_at of ['2027-02-30T18:00:00Z', '2027-03-05T18:00:00', '2027-03-05T24:00:00Z', '2027-03-05T18:00:00.1234567Z']) expect(() => makeConcernMutation('create_risk', parent, { ...values(), due_at }, 'Reason')).toThrow();
  const input = makeConcernMutation('update_risk', record, values(), 'Precise instant'); expect(ack(input)).toBe(true);
  const m = view(input); expect(ack(input, { ...m, result: { ...m.result, due_at: '2027-03-05T10:00:00.123999Z' } })).toBe(false);
});
it('creation receipts require actual new version one, the exact parent and the real sender', () => {
  const input = makeConcernMutation('create_risk', parent, values(), 'Exact create'), m = view(input); expect(ack(input)).toBe(true);
  for (const change of [{ id: parent.id }, { version: 8 }, { project_id: id(12) }, { creator_id: id(13) }, { state: 'mitigating' }, { title: 'Other title' }, { probability: 'low' }]) expect(ack(input, { ...m, result: { ...m.result, ...change } })).toBe(false);
  expect(ack(input, { ...m, resource_type: 'risk' })).toBe(false);
});
it('metadata updates preserve omitted state, original creator and explicit date/probability clearing', () => {
  const input = makeConcernMutation('update_risk', record, { ...values(), due_at: null }, 'Explicit clearing'), m = view(input);
  m.result!.due_at = null; expect(ack(input, m)).toBe(true);
  for (const change of [{ state: 'closed' }, { creator_id: id(6) }, { description: 'Different body' }, { due_at: '2027-03-05T10:00:00Z' }]) expect(ack(input, { ...m, result: { ...m.result, ...change } })).toBe(false);
});
it('realization requires an allowed transition and a separate new linked issue receipt', () => {
  const input = makeConcernMutation('update_risk', record, { ...values(), state: 'realized' }, 'Reviewed realization'), m = view(input);
  expect(ack(input, m)).toBe(false); m.result!.issue_id = id(15); expect(ack(input, m)).toBe(true);
  expect(ack(input, { ...m, result: { ...m.result, issue_id: record.id } })).toBe(false);
  expect(() => makeConcernMutation('update_risk', { ...record, state: 'realized' }, values(), 'Forbidden later edit')).toThrow();
  expect(() => makeConcernMutation('update_risk', { ...record, state: 'closed' }, values(), 'Must reopen first')).toThrow();
});
it('issue resolution and closure require evidence and the original state machine', () => {
  const v: ConcernValues = { title: 'Issue', description: '', owner_id: id(3), severity: 'medium', due_at: null, resolution: '' };
  expect(() => makeConcernMutation('update_issue', record, { ...v, state: 'resolved' }, 'Resolve')).toThrow();
  expect(() => makeConcernMutation('update_issue', record, { ...v, resolution: 'Evidence', state: 'closed' }, 'Skip resolution')).toThrow();
  expect(makeConcernMutation('update_issue', { ...record, state: 'resolved' }, { ...v, resolution: 'Evidence', state: 'closed' }, 'Close')).toBeTruthy();
});
