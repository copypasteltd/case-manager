import assert from 'node:assert/strict';
import { test } from 'node:test';
import { randomUUID } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { loadSkills } from './skills.js';
import { verifyResult, type ExecutionReceipt, type ProposedResult } from './evidence.js';

const catalog = await loadSkills(fileURLToPath(new URL('../../../', import.meta.url)));
test('task creation needs complete human decisions and stays within the project-update Skill', () => {
  const arguments_ = { id: randomUUID(), expected_version: 8, reason: 'Employee reviewed', title: 'Task', body: '', assignee_id: null, parent_id: null, milestone_id: null, start_at: null, due_at: null, priority: 'normal', review_required: false };
  const input = { instructions: 'Only the exact complete task', mutation: { tool: 'create_task', arguments: arguments_ } }, skill = catalog.get('project-update')!;
  skill.validateInput(input); skill.authorizeTool('create_task');
  for (const field of Object.keys(arguments_)) { const copy = structuredClone(input); delete (copy.mutation.arguments as Record<string, unknown>)[field]; assert.throws(() => skill.validateInput(copy), /SKILL_INPUT_INVALID/); }
  for (const patch of [{ review_required: null }, { review_required: 'false' }, { state: 'done' }, { dependency_ids: [randomUUID()] }, { assignee_id: 'guessed-member' }]) assert.throws(() => skill.validateInput({ ...input, mutation: { tool: 'create_task', arguments: { ...arguments_, ...patch } } }), /SKILL_INPUT_INVALID/);
  for (const other of catalog.values()) if (other.manifest.skill_id !== 'project-update') assert.throws(() => other.authorizeTool('create_task'), /SKILL_TOOL_DENIED/);
});
test('task completion requires a current committed receipt for the actual new task', () => {
  const run = { id: randomUUID(), authority_stamp: 'current-task-authority' }, source = { type: 'agent_mutation', id: randomUUID() }, resource = { type: 'task', id: randomUUID() }, operation = randomUUID();
  const receipt: ExecutionReceipt = { run_id: run.id, authority_stamp: run.authority_stamp, tool: 'create_task', status: 'succeeded', committed: true, operation_id: operation, resource_refs: [resource], source_refs: [source] };
  const result: ProposedResult = { status: 'completed', summary: 'The actual reviewed task was created', resource_refs: [resource], source_refs: [source], operation_ids: [operation], missing_fields: [], next_action: '' }, skill = catalog.get('project-update')!;
  assert.deepEqual(verifyResult(skill, run, result, [receipt]), result);
  for (const invalid of [{ ...receipt, run_id: randomUUID() }, { ...receipt, authority_stamp: 'old' }, { ...receipt, committed: false }, { ...receipt, operation_id: null }, { ...receipt, status: 'pending' as const }]) assert.throws(() => verifyResult(skill, run, result, [invalid]), /RESULT_(REFERENCE|OPERATION|COMPLETION)_UNVERIFIED/);
  assert.throws(() => verifyResult(skill, run, { ...result, resource_refs: [{ type: 'task', id: randomUUID() }] }, [receipt]), /RESULT_REFERENCE_UNVERIFIED/);
});
