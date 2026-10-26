import assert from 'node:assert/strict';
import { test } from 'node:test';
import { randomUUID } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { loadSkills } from './skills.js';
import { verifyResult, type ExecutionReceipt, type ProposedResult } from './evidence.js';

const catalog = await loadSkills(fileURLToPath(new URL('../../../', import.meta.url)));
for (const tool of ['create_risk', 'update_risk', 'create_issue', 'update_issue']) {
  test(`${tool} binds complete reviewed fields and current committed business evidence`, () => {
    const risk = tool.endsWith('_risk'), create = tool.startsWith('create_');
    const args: Record<string, unknown> = {
      id: randomUUID(), expected_version: 7, reason: 'Employee reviewed the exact full metadata',
      title: '项目事项', description: '', owner_id: randomUUID(), severity: 'high', due_at: null,
      ...(risk ? { probability: null, mitigation: '' } : { resolution: '' }),
      ...(!create ? { state: risk ? 'realized' : 'investigating' } : {}),
    };
    const skill = catalog.get('project-update')!;
    const input = { instructions: 'Apply only the reviewed command', mutation: { tool, arguments: args } };
    skill.validateInput(input); skill.authorizeTool(tool);
    for (const field of Object.keys(args).filter(key => key !== 'state')) {
      const altered = structuredClone(input); delete altered.mutation.arguments[field];
      assert.throws(() => skill.validateInput(altered), /SKILL_INPUT_INVALID/);
    }
    const changed = (patch: Record<string, unknown>) => ({ ...input, mutation: { tool, arguments: { ...args, ...patch } } });
    for (const patch of [{ severity: 'urgent' }, { title: null }, { owner_id: 'guessed-person' }, { due_at: 'tomorrow' },
      risk ? { resolution: 'cross-kind field' } : { mitigation: 'cross-kind field' }, { state: null },
      ...(create ? [{ state: 'open' }] : [])]) {
      assert.throws(() => skill.validateInput(changed(patch)), /SKILL_INPUT_INVALID/);
    }
    for (const other of catalog.values()) if (other.manifest.skill_id !== 'project-update') {
      assert.throws(() => other.authorizeTool(tool), /SKILL_TOOL_DENIED/);
    }
    const run = { id: randomUUID(), authority_stamp: 'current-concern-authority' };
    const source = { type: 'agent_mutation', id: run.id }, resource = { type: risk ? 'risk' : 'issue', id: randomUUID() };
    const refs = risk && !create ? [resource, { type: 'issue', id: randomUUID() }] : [resource];
    const operation = randomUUID();
    const receipt: ExecutionReceipt = { run_id: run.id, authority_stamp: run.authority_stamp, tool, status: 'succeeded',
      committed: true, operation_id: operation, resource_refs: refs, source_refs: [source] };
    const result: ProposedResult = { status: 'completed', summary: 'Only actual committed resources are cited',
      resource_refs: refs, source_refs: [source], operation_ids: [operation], missing_fields: [], next_action: '' };
    assert.deepEqual(verifyResult(skill, run, result, [receipt]), result);
    for (const invalid of [{ ...receipt, run_id: randomUUID() }, { ...receipt, authority_stamp: 'old-authority' },
      { ...receipt, committed: false }, { ...receipt, operation_id: null }, { ...receipt, status: 'pending' as const }]) {
      assert.throws(() => verifyResult(skill, run, result, [invalid]), /RESULT_(REFERENCE|OPERATION|COMPLETION)_UNVERIFIED/);
    }
    assert.throws(() => verifyResult(skill, run, { ...result, resource_refs: [...refs, { type: 'issue', id: randomUUID() }] }, [receipt]), /RESULT_REFERENCE_UNVERIFIED/);
  });
}
