import assert from 'node:assert/strict';
import { batchInputMatches, checkedFileBatch, fileBatchCreate, fileBatchDecision, type FileBatchPlan } from '../src/file-batch';
import type { components } from '../src/api';
type S = components['schemas'];
const id = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
const root = { kind: 'folder' as const, id: id(3), version: 2, name: '项目资料', parent_id: null, project_id: id(4), classification: 'project_restricted' as const };
const target = { ...root, id: id(5), parent_id: root.id, name: '交付目录', state: 'active', can_create: true } as unknown as S['Folder'];
const source = (n: number) => ({ id: id(n), version: 3, name: '资料' + n + '.txt', folder_id: root.id, project_id: root.project_id, creator_id: id(7), current_version_id: id(n + 10), classification: root.classification, state: 'active', can_move: true, can_trash: true, can_restore: true } as S['File']);
function fixture(): FileBatchPlan {
  const sources = [source(1), source(2)];
  return { id: id(6), creator_id: id(7), version: 1, state: 'pending', plan_hash: 'a'.repeat(64),
    input: fileBatchCreate('move', sources, target, '人工核对后整理'),
    snapshot: { items: sources.map(f => ({ file: { id: f.id, version: f.version, name: f.name, folder_id: f.folder_id, project_id: f.project_id, creator_id: f.creator_id, current_version_id: f.current_version_id, classification: f.classification, state: f.state }, path: [structuredClone(root)], retention_held: false, blocking_reasons: [] })), target: { folder: { ...root, id: target.id, parent_id: root.id, name: target.name }, path: [structuredClone(root)] } },
    created_at: '2026-10-30T00:00:00Z', expires_at: '2026-10-30T00:30:00Z', completed_at: null, decision_reason: null, results: [], current_basis_matches: true, can_execute: true };
}
function done(action: 'move' | 'trash' | 'restore'): FileBatchPlan {
  const p = fixture(); p.input.action = action;
  if (action !== 'move') { p.input.target = null; p.snapshot.target = null; }
  if (action === 'restore') p.snapshot.items.forEach(i => { i.file.state = 'trashed'; });
  p.state = 'executed'; p.version = 2; p.completed_at = '2026-10-30T00:01:00Z'; p.decision_reason = '本人确认'; p.current_basis_matches = false; p.can_execute = false;
  p.results = p.snapshot.items.map(i => ({ ...i.file, version: i.file.version + 1, folder_id: action === 'move' ? target.id : i.file.folder_id, state: action === 'trash' ? 'trashed' : 'active' }));
  return p;
}
type Case = readonly [string, () => void];
const bad: [string, (p: FileBatchPlan) => void][] = [
  ['foreign owner', p => { p.creator_id = id(9); }], ['wrong exact plan', p => { p.id = id(9); }],
  ['missing selected file', p => { p.snapshot.items.pop(); }], ['duplicate source', p => { p.input.files[1] = p.input.files[0]; p.snapshot.items[1] = p.snapshot.items[0]; }],
  ['unsorted membership', p => { p.input.files.reverse(); p.snapshot.items.reverse(); }], ['changed content head', p => { p.snapshot.items[0].file.current_version_id = id(99); }],
  ['changed version', p => { p.snapshot.items[0].file.version++; }], ['wrong target', p => { p.snapshot.target!.folder.id = id(99); }],
  ['unreported state mismatch', p => { p.snapshot.items[0].file.state = 'trashed'; }],
  ['target revision', p => { p.snapshot.target!.folder.version++; }], ['broken source path', p => { p.snapshot.items[0].path[0].id = id(99); }],
  ['cyclic target path', p => { p.snapshot.target!.path.push(root); }], ['wrong project', p => { p.snapshot.items[0].file.project_id = id(99); }],
  ['unrecognized blocker', p => { p.snapshot.items[0].blocking_reasons = ['silently-ignore'] as never; }], ['blocked but executable', p => { p.snapshot.items[0].blocking_reasons = ['name_conflict']; }],
  ['changed basis but executable', p => { p.current_basis_matches = false; }], ['unbounded expiry', p => { p.expires_at = '2026-11-30T00:00:00Z'; }],
  ['hidden result on pending plan', p => { p.results = done('move').results; }], ['hidden target on trash', p => { p.input.action = 'trash'; }],
];
export const fileBatchCases: Case[] = [
  ['analysis pins a sorted unique selection without mutating selected files', () => { const a = source(2), b = source(1), p = fileBatchCreate('move', [a, b], target, '核对'); assert.deepEqual(p.files.map(f => f.id), [b.id, a.id]); assert.equal(a.folder_id, root.id); assert.ok(checkedFileBatch(fixture(), id(7), id(6), id(1)).can_execute); }],
  ...bad.map(([name, mutate]) => [name + ' rejects a mismatched batch', () => { const p = fixture(); mutate(p); assert.throws(() => checkedFileBatch(p, id(7), id(6), id(1))); }] as Case),
  ...(['move', 'trash', 'restore'] as const).map(action => [action + ' validates every original head and exact result', () => {
    const p = done(action); assert.equal(checkedFileBatch(p, id(7)).results.length, 2);
    for (const field of ['id', 'version', 'name', 'folder_id', 'project_id', 'creator_id', 'classification', 'current_version_id', 'state']) { const q = structuredClone(p); (q.results[1] as unknown as Record<string, unknown>)[field] = field === 'version' ? 99 : 'invalid'; assert.throws(() => checkedFileBatch(q, id(7))); }
    const q = structuredClone(p); q.results.pop(); assert.throws(() => checkedFileBatch(q, id(7)));
  }] as Case),
  ['cancel has no file results and one immutable decision version', () => { const p = fixture(); p.state = 'cancelled'; p.version = 2; p.can_execute = false; p.current_basis_matches = false; p.completed_at = '2026-10-30T00:02:00Z'; p.decision_reason = '撤销'; assert.equal(checkedFileBatch(p, id(7)).state, 'cancelled'); p.results = done('move').results; assert.throws(() => checkedFileBatch(p, id(7))); }],
  ['creation rejects no selection duplicates excess files stale permissions and cross scope', () => {
    for (const selected of [[], [source(1), source(1)], Array.from({ length: 51 }, (_, n) => source(n + 20)), [{ ...source(1), can_trash: false }], [{ ...source(1), state: 'trashed' as const }]]) assert.throws(() => fileBatchCreate('trash', selected, undefined, '整理'));
    for (const destination of [undefined, { ...target, can_create: false }, { ...target, classification: 'internal' as const }, { ...target, project_id: null }, { ...target, id: root.id }]) assert.throws(() => fileBatchCreate('move', [source(1)], destination, '整理'));
    assert.throws(() => fileBatchCreate('trash', [source(1)], undefined, ' '.repeat(20))); assert.throws(() => fileBatchCreate('trash', [source(1)], undefined, 'a'.repeat(201)));
  }],
  ['decisions require a current executable plan explicit acknowledgement and fresh reason', () => {
    const p = fixture(); assert.deepEqual(fileBatchDecision(p, '本人确认', true, 'execute'), { expected_version: 1, plan_hash: p.plan_hash, reason: '本人确认', acknowledged: true });
    assert.throws(() => fileBatchDecision(p, '核对', false, 'execute')); assert.throws(() => fileBatchDecision(p, '', true, 'execute'));
    p.can_execute = false; assert.throws(() => fileBatchDecision(p, '核对', true, 'execute')); assert.equal(fileBatchDecision(p, '撤销', true, 'cancel').acknowledged, true);
    assert.throws(() => fileBatchDecision(done('trash'), '重复', true, 'execute'));
  }],
  ['receipt comparison covers the exact action selected versions content heads target and reason', () => {
    const p = fixture(); assert.ok(batchInputMatches(p.input, structuredClone(p.input)));
    for (const mutate of [(v: typeof p.input) => { v.action = 'trash'; }, (v: typeof p.input) => { v.files[0].expected_version++; }, (v: typeof p.input) => { v.files[0].current_version_id = id(99); }, (v: typeof p.input) => { v.target!.expected_version++; }, (v: typeof p.input) => { v.reason += 'changed'; }]) { const copy = structuredClone(p.input); mutate(copy); assert.equal(batchInputMatches(p.input, copy), false); }
  }],
];
