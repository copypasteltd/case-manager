import assert from 'node:assert/strict';
import { checkedScopeTransfer, scopeTransferCreate, scopeTransferDecision, type ScopeTransfer, type TransferResource } from '../src/scope-transfer';
import type { components } from '../src/api';
type Schemas = components['schemas'];
const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
function fixture(): ScopeTransfer {
  const source = { kind: 'file' as const, id: id(1), version: 3, name: '项目 <script>文本🙂.txt', parent_id: id(2), project_id: id(3), classification: 'project_restricted' as const };
  const target = { kind: 'folder' as const, id: id(4), version: 2, name: '目标项目资料', parent_id: null, project_id: id(5), classification: 'project_restricted' as const };
  return { id: id(6), version: 1, state: 'pending', creator_id: id(7), plan_hash: 'a'.repeat(64), created_at: '2026-10-30T00:00:00Z', expires_at: '2026-10-30T00:30:00Z', completed_at: null, decision_reason: null, current_basis_matches: true, can_execute: true,
    input: { kind: 'file', resource_id: source.id, expected_version: 3, target_folder_id: target.id, expected_target_version: 2, new_name: null, reason: '人工跨项目整理' },
    snapshot: { source, target, new_name: source.name, source_path: [{ ...target, id: id(2), project_id: id(3) }], target_path: [], items: [{ ...source, state: 'active', creator_id: id(7), new_classification: target.classification, current_version_id: id(8), content_versions: 2, content_sha256: 'b'.repeat(64), human_before: 5, human_after: 5, ai_before: 5, ai_after: 5, audience_sha256: 'c'.repeat(64), blocking_reasons: [], visibility_changes: [{ user_id: id(9), display_name: '旧项目员工', channel: 'web', ai: false, before: true, after: false }, { user_id: id(10), display_name: '新项目员工', channel: 'web', ai: false, before: false, after: true }] }] } };
}
const check = (p: ScopeTransfer) => checkedScopeTransfer(p, 'file', id(1), id(6));
type Case = readonly [string, () => void];
const rejects = (name: string, mutate: (p: ScopeTransfer) => void): Case => [name, () => { const p = fixture(); mutate(p); assert.throws(() => check(p)); }];
function resources() {
  const source = { id: id(1), version: 3, state: 'active', name: '资料.txt', project_id: id(3), classification: 'project_restricted', can_move: true } as Schemas['File'];
  const target = { id: id(4), version: 2, state: 'active', name: '目标', parent_id: null, project_id: id(5), classification: 'project_restricted', can_create: true } as Schemas['Folder'];
  return { source, target };
}
export const scopeTransferCases: readonly Case[] = [
  ['fixed exact file inventory preserves literal names and policy changes', () => { const p = fixture(); assert.equal(check(p), p); assert.deepEqual(scopeTransferDecision(p, '核对后确认', true, 'execute'), { expected_version: 1, plan_hash: p.plan_hash, reason: '核对后确认' }); }],
  ['blocked pending plan may be read but cannot execute', () => { const p = fixture(); p.can_execute = false; p.snapshot.items[0].blocking_reasons = ['BUSINESS_REFERENCE']; check(p); assert.throws(() => scopeTransferDecision(p, '检查', true, 'execute')); assert.equal(scopeTransferDecision(p, '撤销', true, 'cancel').plan_hash, p.plan_hash); }],
  ['final immutable receipt includes the exact hash and human reason', () => { const p = fixture(); p.state = 'executed'; p.version = 2; p.can_execute = false; p.current_basis_matches = false; p.completed_at = '2026-10-30T00:10:00Z'; p.decision_reason = '已核对'; check(p); assert.throws(() => scopeTransferDecision(p, '重复', true, 'execute')); }],
  ['proposal preserves both actual versions and explicit new name', () => { const { source, target } = resources(); const p = scopeTransferCreate('file', source, target, '整理后🙂.txt', '实际项目整理'); assert.equal(p.expected_version, 3); assert.equal(p.expected_target_version, 2); assert.equal(p.new_name, '整理后🙂.txt'); }],
  ['proposal rejects path tricks reserved names and unicode normalization bypasses', () => { const { source, target } = resources(); for (const n of ['../x', 'Ａ／Ｂ', 'con.txt', 'nul', '尾部.', 'x\u202ey', ' x']) assert.throws(() => scopeTransferCreate('file', source, target, n, '明确原因')); }],
  ['same scope uses ordinary moves and fixed root folders cannot transfer', () => { const { source, target } = resources(); assert.throws(() => scopeTransferCreate('file', source, { ...target, project_id: source.project_id }, '文件.txt', '明确原因')); const folder = { ...target, can_update: true } as TransferResource; assert.throws(() => scopeTransferCreate('folder', folder, { ...target, id: id(11) }, '目录', '明确原因')); }],
  ['missing actual mutation permissions cannot be replaced by form data', () => { const { source, target } = resources(); assert.throws(() => scopeTransferCreate('file', { ...source, can_move: false }, target, '文件.txt', '明确原因')); assert.throws(() => scopeTransferCreate('file', source, { ...target, can_create: false }, '文件.txt', '明确原因')); }],
  ['explicit acknowledgement and a bounded reason are mandatory', () => { assert.throws(() => scopeTransferDecision(fixture(), '确认', false, 'execute')); assert.throws(() => scopeTransferDecision(fixture(), '', true, 'execute')); assert.throws(() => scopeTransferDecision(fixture(), 'x'.repeat(201), true, 'execute')); }],
  rejects('rejects foreign resource and parent version', p => { p.snapshot.source.version++; }),
  rejects('rejects incomplete fixed inventory', p => { p.snapshot.items = []; }),
  rejects('rejects unbounded directory inventory', p => { p.snapshot.items = Array.from({ length: 101 }, () => p.snapshot.items[0]); }),
  rejects('rejects duplicate inventory identities', p => { p.snapshot.items.push(p.snapshot.items[0]); }),
  rejects('rejects target folder version mismatch', p => { p.input.expected_target_version++; }),
  rejects('rejects different planned name', p => { p.snapshot.new_name = '未确认的名称'; }),
  rejects('rejects mixed source project inventory', p => { p.snapshot.items[0].project_id = id(99); }),
  rejects('rejects invented content history digest', p => { p.snapshot.items[0].content_sha256 = ''; }),
  rejects('rejects unknown hidden blocker', p => { p.snapshot.items[0].blocking_reasons.push('UNRECOGNIZED'); }),
  rejects('rejects a blocked executable claim', p => { p.snapshot.items[0].blocking_reasons.push('RETENTION_HOLD'); }),
  rejects('rejects stale executable claim', p => { p.current_basis_matches = false; }),
  rejects('rejects changed policy counts without matching changes', p => { p.snapshot.items[0].human_after++; }),
  rejects('rejects duplicate audience change combinations', p => { p.snapshot.items[0].visibility_changes.push(p.snapshot.items[0].visibility_changes[0]); }),
  rejects('rejects unchanged row disguised as a visibility change', p => { p.snapshot.items[0].visibility_changes[0].after = true; }),
  rejects('rejects arbitrary channel authorization claims', p => { p.snapshot.items[0].visibility_changes[0].channel = 'public-link' as 'web'; }),
  rejects('rejects final status without a human completion receipt', p => { p.state = 'executed'; p.can_execute = false; }),
];
