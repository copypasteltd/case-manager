import assert from 'node:assert/strict';
import { checkedWorkbench, workbenchURL, type WorkbenchSummary } from '../src/workbench';
import { checkedReplicaRetry, replicaRetryInput } from '../src/file-replica';
import type { components } from '../src/api';
const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
function fixture(): WorkbenchSummary {
  const item = { id: id(1), version: 2, revision_id: id(2), related_id: id(3), title: '香港 <script>literal</script> 🙂', state: 'active' as const, due_on: null, updated_at: '2026-10-30T00:00:00Z' };
  return { as_of: '2026-10-29T17:00:00Z', business_date: '2026-10-30', contract_through: '2026-11-29', contract_timezone: 'Asia/Hong_Kong', scope: 'my_readable_work', groups: [
    { kind: 'approvals', count: 1, has_more: false, items: [item] },
    { kind: 'contracts', count: 1, has_more: false, items: [{ ...item, id: id(4), related_id: null, state: 'overdue', due_on: '2026-10-29' }] },
    { kind: 'subscription_periods', count: 0, has_more: false, items: [] },
    { kind: 'incidents', count: 0, has_more: false, items: [] },
    { kind: 'file_attention', count: 0, has_more: false, items: [] },
  ] };
}
type Case = readonly [string, () => void];
const rejects = (name: string, change: (v: WorkbenchSummary) => void): Case => [name, () => { const v = fixture(); change(v); assert.throws(() => checkedWorkbench(v)); }];
const replica = (): components['schemas']['FileReplica'] => ({ id: id(50), version: 3, provider: 'r2', state: 'failed', actual_sha256: null, verified_at: null, error_code: 'REPLICA_FAILED', updated_at: '2026-10-30T00:00:00Z' });
const retryReceipt = () => ({ ...replica(), version: 4, state: 'queued' as const, error_code: null });
export const workbenchCases: readonly Case[] = [
  ['Hong Kong date and exact scoped counts preserve literal titles', () => { const v = fixture(); assert.equal(checkedWorkbench(v), v); assert.match(workbenchURL('approvals', v.groups[0].items[0], 'web'), /^\/requests\//); assert.match(workbenchURL('contracts', v.groups[1].items[0], 'mobile'), /^\/pages\/business\/index\?kind=contract&id=/); assert.match(workbenchURL('subscription_periods', v.groups[0].items[0], 'mobile'), /^\/pages\/subscription-period\/index\?id=/); }],
  ['empty current readable work is valid', () => { const v = fixture(); for (const g of v.groups) { g.count = 0; g.items = []; } checkedWorkbench(v); }],
  ['more than five rows requires complete count and bounded ordered samples', () => { const v = fixture(), g = v.groups[0], first = g.items[0]; g.items = Array.from({ length: 5 }, (_, i) => ({ ...first, id: id(20 - i) })); g.count = 7; g.has_more = true; checkedWorkbench(v); }],
  rejects('rejects missing domain even when every surviving row is valid', v => { v.groups.pop(); }),
  rejects('rejects duplicate domain', v => { v.groups[2] = v.groups[0]; }),
  rejects('rejects organization-wide or other scope label', v => { v.scope = 'all_work' as WorkbenchSummary['scope']; }),
  rejects('rejects wrong business date around UTC midnight', v => { v.business_date = '2026-10-29'; }),
  rejects('rejects normalized invalid calendar date', v => { v.business_date = '2026-02-30'; }),
  rejects('rejects changed fixed contract horizon', v => { v.contract_through = '2026-11-28'; }),
  rejects('rejects guessed zero when a row exists', v => { v.groups[0].count = 0; }),
  rejects('rejects missing sampled rows beneath a positive count', v => { v.groups[0].count = 2; }),
  rejects('rejects a false complete-list claim', v => { v.groups[0].has_more = true; }),
  rejects('rejects foreign route text posing as an identifier', v => { v.groups[0].items[0].id = '//evil.example'; }),
  rejects('rejects approval without exact active-step context', v => { v.groups[0].items[0].related_id = null; }),
  rejects('rejects contract copied from an unbounded later expiry', v => { v.groups[1].items[0].due_on = '2026-11-30'; v.groups[1].items[0].state = 'due_soon'; }),
  rejects('rejects a paid or overdue claim in a billing review group', v => { v.groups[2].count = 1; v.groups[2].items = [{ ...v.groups[1].items[0], related_id: id(7) }]; }),
  rejects('rejects unstable duplicate sample identity', v => { v.groups[0].items.push(v.groups[0].items[0]); v.groups[0].count = 2; }),
  rejects('rejects reversed due-date ordering', v => { const g = v.groups[1]; g.items.push({ ...g.items[0], id: id(8), due_on: '2026-10-28' }); g.count = 2; }),
  ['assigned incidents keep exact timeline context and both destinations', () => { const v = fixture(), g = v.groups[3]; g.count = 1; g.items = [{ ...v.groups[0].items[0], id: id(9), related_id: null, state: 'investigating' }]; checkedWorkbench(v); assert.equal(workbenchURL('incidents', g.items[0], 'web'), '/incidents/' + id(9)); assert.equal(workbenchURL('incidents', g.items[0], 'mobile'), '/pages/status/index?kind=incident&id=' + id(9)); }],
  rejects('resolved incidents cannot remain in assigned open work', v => { const g = v.groups[3]; g.count = 1; g.items = [{ ...v.groups[0].items[0], related_id: null, state: 'resolved' as 'active' }]; }),
  rejects('incident work cannot fabricate a due date', v => { const g = v.groups[3]; g.count = 1; g.items = [{ ...v.groups[0].items[0], related_id: null, state: 'open', due_on: '2026-10-30' }]; }),
  ['file work preserves the exact current head and both destinations', () => { const v = fixture(), g = v.groups[4]; g.count = 1; g.items = [{ ...v.groups[0].items[0], related_id: null, state: 'replica_pending' }]; checkedWorkbench(v); assert.equal(workbenchURL('file_attention', g.items[0], 'web'), '/files/' + id(1)); assert.equal(workbenchURL('file_attention', g.items[0], 'mobile'), '/pages/files/index?file_id=' + id(1)); }],
  ['unavailable and failed replica work are distinct valid states', () => { for (const state of ['file_unavailable', 'replica_attention'] as const) { const v = fixture(), g = v.groups[4]; g.count = 1; g.items = [{ ...v.groups[0].items[0], related_id: null, state }]; checkedWorkbench(v); } }],
  rejects('a healthy file cannot remain in attention work', v => { const g = v.groups[4]; g.count = 1; g.items = [{ ...v.groups[0].items[0], related_id: null, state: 'verified' as 'active' }]; }),
  rejects('file work cannot substitute an approval step for its head', v => { const g = v.groups[4]; g.count = 1; g.items = [{ ...v.groups[0].items[0], state: 'replica_attention' }]; }),
  ['retry pins the selected revision and clears obsolete verification evidence', () => { const before = replica(); assert.deepEqual(replicaRetryInput(before, ' 修复后重试 '), { expected_version: 3, reason: '修复后重试' }); const value = retryReceipt(); assert.equal(checkedReplicaRetry(value, before), value); assert.equal(before.state, 'failed'); }],
  ['retry refuses healthy active or unsafe resource revisions', () => { for (const changed of [{ state: 'verified' }, { state: 'copying' }, { id: '../escape' }, { version: 0 }, { version: Number.MAX_SAFE_INTEGER }]) assert.throws(() => replicaRetryInput({ ...replica(), ...changed } as ReturnType<typeof replica>, 'explicit')); }],
  ['retry reason must be present and bounded in Unicode characters', () => { assert.throws(() => replicaRetryInput(replica(), ' ')); assert.throws(() => replicaRetryInput(replica(), '🙂'.repeat(201))); assert.equal([...replicaRetryInput(replica(), '🙂'.repeat(200)).reason].length, 200); }],
  ['retry receipt rejects a different replica provider or revision', () => { for (const changed of [{ id: id(51) }, { provider: 'oss' }, { version: 3 }, { version: 5 }]) assert.throws(() => checkedReplicaRetry({ ...retryReceipt(), ...changed } as ReturnType<typeof replica>, replica())); }],
  ['a retry receipt cannot claim completion or retain old hashes errors or verification time', () => { for (const changed of [{ state: 'verified' }, { actual_sha256: '0'.repeat(64) }, { verified_at: '2026-10-30T00:00:00Z' }, { error_code: 'REPLICA_FAILED' }]) assert.throws(() => checkedReplicaRetry({ ...retryReceipt(), ...changed } as ReturnType<typeof replica>, replica())); }],
  ['retry receipt rejects malformed update time', () => { assert.throws(() => checkedReplicaRetry({ ...retryReceipt(), updated_at: 'invalid' }, replica())); }],
  ['route builders reject untrusted domain and resource inputs', () => { const item = fixture().groups[0].items[0]; assert.throws(() => workbenchURL('other' as 'approvals', item, 'web')); assert.throws(() => workbenchURL('approvals', { ...item, id: '../wrong' }, 'mobile')); }],
];
