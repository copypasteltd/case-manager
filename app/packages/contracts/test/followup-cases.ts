import assert from 'node:assert/strict';
import { checkedFollowups, followupState, followupURL, type FollowupSummary } from '../src/followups';
const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
function fixture(): FollowupSummary {
  const item = { id: id(1), version: 2, revision_id: id(2), project_id: id(3), title: '合成 <script>literal</script> 🙂', state: 'returned' as const, updated_at: '2026-10-31T00:00:00Z' };
  return { scope: 'my_readable_followups', as_of: '2026-10-31T00:01:00Z', groups: [
    { kind: 'requests_to_revise', count: 1, has_more: false, items: [item] },
    { kind: 'deliverables_to_submit', count: 1, has_more: false, items: [{ ...item, id: id(4), state: 'draft', revision_id: null }] },
    { kind: 'deliverables_to_review', count: 1, has_more: false, items: [{ ...item, id: id(5), state: 'submitted' }] },
  ] };
}
type Case = readonly [string, () => void];
export const followupCases: Case[] = [
  ['current exact metadata distinguishes unsubmitted null revision and independent submitted review', () => {
    const v = fixture(); assert.equal(checkedFollowups(v), v); assert.equal(v.groups[1].items[0].revision_id, null);
    assert.equal(followupState(v.groups[0].items[0]), '已退回，待修订'); assert.match(v.groups[0].items[0].title, /<script>literal<\/script>/);
    v.groups[1].items[0].state = 'changes_requested'; v.groups[1].items[0].revision_id = id(7); checkedFollowups(v);
  }],
  ['incomplete counts, stale states, hidden payload fields and malformed source identities fail closed', () => {
    const changes: Array<(v: FollowupSummary) => void> = [
      v => { v.groups.pop(); }, v => { v.groups[1].kind = 'requests_to_revise'; },
      v => { v.groups[0].count = 2; }, v => { v.groups[0].has_more = true; }, v => { v.groups[0].count = Number.MAX_SAFE_INTEGER + 1; },
      v => { v.groups[0].items[0].revision_id = null; }, v => { v.groups[1].items[0].project_id = null; }, v => { v.groups[2].items[0].revision_id = null; },
      v => { v.groups[0].items[0].state = 'submitted'; }, v => { v.groups[2].items[0].state = 'draft'; },
      v => { v.groups[0].items[0].version = 0; }, v => { v.groups[0].items[0].id = '../other'; }, v => { v.groups[0].items[0].updated_at = 'unknown'; },
      v => { Object.assign(v.groups[0].items[0], { payload: { amount: '123.45' } }); },
      v => { v.groups[2].items[0].id = v.groups[1].items[0].id; }, v => { v.groups[0].items[0].title = ' '; },
      v => { Object.assign(v, { scope: 'all_company_work' }); }, v => { Object.assign(v.groups[0], { cursor: 'untrusted' }); },
    ];
    for (const change of changes) { const v = fixture(); change(v); assert.throws(() => checkedFollowups(v)); }
  }],
  ['complete count and newest five records retain deterministic ordering and honest more indicator', () => {
    const v = fixture(), item = v.groups[0].items[0];
    v.groups[0] = { kind: 'requests_to_revise', count: 7, has_more: true, items: [9, 8, 7, 6, 5].map(n => ({ ...item, id: id(n) })) };
    checkedFollowups(v); v.groups[0].items.reverse(); assert.throws(() => checkedFollowups(v));
    const empty = fixture(); for (const g of empty.groups) { g.count = 0; g.items = []; } checkedFollowups(empty);
  }],
  ['Web and shared mobile routes lead to ordinary request or deliverable details using only verified IDs', () => {
    const v = checkedFollowups(fixture());
    for (const group of v.groups) {
      const item = group.items[0];
      assert.equal(followupURL(group.kind, item, 'web'), group.kind === 'requests_to_revise' ? '/requests/' + item.id : '/projects/' + item.project_id + '?tab=deliverables&deliverable=' + item.id);
      assert.equal(followupURL(group.kind, item, 'mobile'), (group.kind === 'requests_to_revise' ? '/pages/detail/index?kind=request&id=' : '/pages/deliverable/index?id=') + item.id);
    }
    assert.throws(() => followupURL('untrusted' as never, v.groups[0].items[0], 'web'));
    assert.throws(() => followupURL('requests_to_revise', { ...v.groups[0].items[0], id: 'https://other.invalid/' }, 'mobile'));
  }],
];
