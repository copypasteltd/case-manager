import assert from 'node:assert/strict';
import { test } from 'node:test';
import { randomUUID } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { loadSkills } from './skills.js';
import { verifyResult, type ExecutionReceipt, type ProposedResult } from './evidence.js';

const catalog = await loadSkills(fileURLToPath(new URL('../../../', import.meta.url)));
test('only project-update accepts a complete exact dependency authority', () => {
  const args = { id: randomUUID(), expected_version: 4, predecessor_id: randomUUID(), expected_predecessor_version: 7, action: 'add', reason: 'Employee reviewed the exact pair' };
  const skill = catalog.get('project-update')!, input = { instructions: 'Change only the exact selected relationship', mutation: { tool: 'update_task_dependency', arguments: args } };
  skill.authorizeTool('update_task_dependency'); skill.validateInput(input);
  assert.equal(skill.manifest.state, 'draft'); assert.equal(skill.manifest.version, '1.8.0');
  for (const key of Object.keys(args)) { const copy = structuredClone(input); delete (copy.mutation.arguments as Record<string, unknown>)[key]; assert.throws(() => skill.validateInput(copy), /SKILL_INPUT_INVALID/); }
  for (const patch of [{ action: 'replace' }, { expected_predecessor_version: 0 }, { predecessor_id: null }, { dependencies: [randomUUID()] }, { review_required: false }]) assert.throws(() => skill.validateInput({ ...input, mutation: { ...input.mutation, arguments: { ...args, ...patch } } }), /SKILL_INPUT_INVALID/);
  for (const other of catalog.values()) if (other.manifest.skill_id !== 'project-update') assert.throws(() => other.authorizeTool('update_task_dependency'), /SKILL_TOOL_DENIED/);
});
test('dependency completion uses the current trusted operation receipt', () => {
  const run = { id: randomUUID(), authority_stamp: 'current-dependency-authority' }, source = { type: 'agent_mutation', id: run.id }, resource = { type: 'task', id: randomUUID() }, operation = randomUUID();
  const receipt: ExecutionReceipt = { run_id: run.id, authority_stamp: run.authority_stamp, tool: 'update_task_dependency', status: 'succeeded', committed: true, operation_id: operation, resource_refs: [resource], source_refs: [source] };
  const proposed: ProposedResult = { status: 'completed', summary: 'The exact selected dependency was changed', resource_refs: [resource], source_refs: [source], operation_ids: [operation], missing_fields: [], next_action: '' }, skill = catalog.get('project-update')!;
  assert.deepEqual(verifyResult(skill, run, proposed, [receipt]), proposed);
  for (const patch of [{ run_id: randomUUID() }, { authority_stamp: 'old' }, { committed: false }, { operation_id: null }, { status: 'pending' as const }]) assert.throws(() => verifyResult(skill, run, proposed, [{ ...receipt, ...patch }]), /RESULT_(REFERENCE|OPERATION|COMPLETION)_UNVERIFIED/);
});
