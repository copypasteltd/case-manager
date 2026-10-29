import assert from 'node:assert/strict';
import { checkedWorkbench, workbenchURL, type WorkbenchSummary } from '../src/workbench';
const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
function fixture(): WorkbenchSummary {
  const item = { id: id(1), version: 2, revision_id: id(2), related_id: id(3), title: '香港 <script>literal</script> 🙂', state: 'active' as const, due_on: null, updated_at: '2026-10-30T00:00:00Z' };
  return { as_of: '2026-10-29T17:00:00Z', business_date: '2026-10-30', contract_through: '2026-11-29', contract_timezone: 'Asia/Hong_Kong', scope: 'my_readable_work', groups: [
    { kind: 'approvals', count: 1, has_more: false, items: [item] },
    { kind: 'contracts', count: 1, has_more: false, items: [{ ...item, id: id(4), related_id: null, state: 'overdue', due_on: '2026-10-29' }] },
    { kind: 'subscription_periods', count: 0, has_more: false, items: [] },
  ] };
}
type Case = readonly [string, () => void];
const rejects = (name: string, change: (v: WorkbenchSummary) => void): Case => [name, () => { const v = fixture(); change(v); assert.throws(() => checkedWorkbench(v)); }];
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
  ['route builders reject untrusted domain and resource inputs', () => { const item = fixture().groups[0].items[0]; assert.throws(() => workbenchURL('other' as 'approvals', item, 'web')); assert.throws(() => workbenchURL('approvals', { ...item, id: '../wrong' }, 'mobile')); }],
];
