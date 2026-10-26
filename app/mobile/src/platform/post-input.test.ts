import test from 'node:test';
import assert from 'node:assert/strict';
import { commentPayload, commentReceipt, completePost, postHash, postInput, postPayload, postReceipt, postSources, restoreReceipt, revisionReceipt, transitionPayload, transitionReceipt, validPost, type Post, type Revision } from './post-input';
import type { Schemas } from './session';

const id = (n: number) => '00000000-0000-0000-0000-' + String(n).padStart(12, '0');
const author = id(2), fixed = id(8);
function post(overrides: Partial<Post> = {}): Post {
  const value: Post = { id: id(1), version: 1, created_at: '2026-10-27T00:00:00Z', updated_at: '2026-10-27T00:00:00Z', project_id: id(3), author_id: author,
    type: 'meeting_note', classification: 'project_restricted', title: '会议纪要 & <release>', body: '# 下一步\n中文 🚀', summary: '已核对\u2028正文', tags: ['会议', 'Release'],
    source_refs: [{ type: 'file', id: id(7) }, { type: 'file', id: id(7), version_id: fixed }], current_revision_id: id(4), content_hash: '', published_revision_id: null,
    has_unpublished_changes: true, can_edit: true, can_publish: true, can_read_history: true, can_comment: true, state: 'draft', ...overrides };
  value.content_hash = postHash(value); return value;
}
test('complete content detects filtered sources, preserves reference order and distinguishes follow from fixed', () => {
  const p = post(); assert(completePost(p)); assert(!completePost({ ...p, source_refs: p.source_refs.slice(0, 1) }));
  assert(!completePost({ ...p, source_refs: [...p.source_refs].reverse() }));
  assert.equal(postHash({ ...p, source_refs: [{ type: 'file', id: id(7), version_id: null }, p.source_refs[1]] }), p.content_hash);
  assert.deepEqual(postSources([p.source_refs[0], p.source_refs[0]]), [p.source_refs[0], p.source_refs[0]]);
});
test('all four knowledge types accept company/project drafts, Unicode and the full 100-reference limit', () => {
  for (const type of ['project_log', 'meeting_note', 'sop', 'article'] as const) {
    const input = { ...postInput(post()), type, project: '', classification: 'internal' as const, title: '🚀'.repeat(200), sources: Array.from({ length: 100 }, (_, i) => ({ type: 'file', id: id(i + 20) })) };
    const payload = postPayload(input); assert.equal(payload.project_id, null); assert.equal(payload.source_refs?.length, 100);
    assert.throws(() => postPayload({ ...input, title: input.title + '字' }));
    assert.throws(() => postPayload({ ...input, sources: [...input.sources, input.sources[0]] }));
  }
});
test('edits bind exact version/base revision, reject incomplete source sets and cannot change scope/type/class', () => {
  const p = post(), input = postInput(p), payload = postPayload(input, p) as Schemas['PostPatch'];
  assert.equal(payload.expected_version, p.version); assert.equal(payload.base_revision_id, p.current_revision_id);
  for (const changed of [{ project: '' }, { type: 'article' as const }, { classification: 'internal' as const }, { tags: 'Release,release' }, { tags: 'a,,b' }, { body: '\ud800' }]) assert.throws(() => postPayload({ ...input, ...changed }, p));
  assert.throws(() => postPayload(input, { ...p, source_refs: [] }));
  const legacy = post({ tags: ['原始,标签', '原始，标签'] });
  assert.deepEqual(postPayload({ ...postInput(legacy), body: '仅改正文' }, legacy).tags, legacy.tags);
});
test('create and edit receipts reject wrong identity, bodies, publication pointers and missing fields', () => {
  const before = post(), input = { ...postInput(before), body: '新正文' }, next = post({ body: input.body, version: 2, current_revision_id: id(5) });
  assert(postReceipt(next, input, author, before));
  for (const changed of [{ version: 1 }, { id: id(99) }, { current_revision_id: before.current_revision_id }, { body: '错正文' }, { published_revision_id: id(6) }, { source_refs: [] }, { author_id: id(55) }]) assert(!postReceipt({ ...next, ...changed }, input, author, before));
  assert(!validPost({ ...next, type: 'script' })); assert(!validPost({ ...next, can_edit: undefined }));
  assert(postReceipt(before, postInput(before), author));
});
test('publication requires content/intent/reason and draft/archive keep the published pointer', () => {
  const p = post(); assert.throws(() => transitionPayload(p, 'publish', '', true)); assert.throws(() => transitionPayload(p, 'publish', '核对', false));
  assert.throws(() => transitionPayload(p, 'archive', '核对', true));
  const published = { ...p, state: 'published' as const, version: 2, published_revision_id: p.current_revision_id, has_unpublished_changes: false };
  assert(transitionReceipt(published, p, 'publish'));
  assert(transitionReceipt({ ...published, state: 'draft', version: 3 }, published, 'draft'));
  assert(!transitionReceipt({ ...published, state: 'draft', version: 3, published_revision_id: null }, published, 'draft'));
  assert.throws(() => transitionPayload({ ...p, source_refs: [] }, 'publish', '核对完整正文', true));
});
test('restoration receipts must establish a new revision and corroborate the immutable history base', () => {
  const before = post({ current_revision_id: id(5), state: 'published', published_revision_id: id(4) });
  const revision: Revision = { id: id(4), post_id: before.id, seq: 1, base_revision_id: null, title: '原始标题', body: '原始正文', summary: '', tags: [], source_refs: [], author_id: author, created_at: before.created_at };
  const next = post({ ...before, title: revision.title, body: revision.body, summary: '', tags: [], source_refs: [], version: 2, current_revision_id: id(6) });
  assert(restoreReceipt(next, before, revision));
  assert(!restoreReceipt({ ...next, current_revision_id: revision.id }, before, revision));
  assert(!restoreReceipt({ ...next, published_revision_id: next.current_revision_id }, before, revision));
  const latest = { ...revision, id: next.current_revision_id, seq: 3, base_revision_id: before.current_revision_id };
  assert(revisionReceipt(latest, next, before)); assert(!revisionReceipt({ ...latest, base_revision_id: id(99) }, next, before));
});
function comment(): Schemas['Comment'] { return { id: id(10), resource_type: 'post', resource_id: id(1), author_id: author, author_display_name: '合成员工', body: '原评论', revision: 1, attachment_versions: [fixed], replaces_id: null, context_revision_id: id(4), created_at: '2026-10-27T00:00:00Z' }; }
test('text-only comment edits omit attachment replacement; an explicit empty list remains an explicit clear', () => {
  const p = post(), before = comment();
  const text = commentPayload(p, '修改文字', undefined, before); assert(!Object.prototype.hasOwnProperty.call(text, 'attachment_versions'));
  assert.equal(text.expected_context_revision_id, p.current_revision_id);
  assert.deepEqual(commentPayload(p, '明确清空', [], before).attachment_versions, []);
  assert.throws(() => commentPayload(p, '评论', [fixed, fixed]));
  assert.throws(() => commentPayload(p, '评论', Array.from({ length: 21 }, (_, i) => id(i + 30))));
  assert.throws(() => commentPayload({ ...p, can_comment: false }, '评论', []));
});
test('comment receipts require a new ID, exact context, preserved pins and immutable predecessor', () => {
  const p = post(), before = comment(), next = { ...before, id: id(11), body: '新评论', revision: 2, replaces_id: before.id };
  assert(commentReceipt(next, p, author, next.body, [fixed], before));
  for (const changed of [{ id: before.id }, { context_revision_id: id(9) }, { attachment_versions: [] }, { replaces_id: null }, { revision: 1 }, { body: '错评论' }]) assert(!commentReceipt({ ...next, ...changed }, p, author, next.body, [fixed], before));
});
