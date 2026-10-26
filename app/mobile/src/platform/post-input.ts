import { sha256 } from '@noble/hashes/sha2.js';
import { bytesToHex, utf8ToBytes } from '@noble/hashes/utils.js';
import { MobileError, type Schemas } from './session';

export type Post = Required<Schemas['Post']>;
export type Revision = Required<Schemas['PostRevision']>;
export type Source = Schemas['ResourceRef'];
export const postTypes = { project_log: '项目日志', meeting_note: '会议纪要', sop: '操作规范', article: '内部文章' };
export const postClasses = { internal: '公司内部', project_restricted: '项目受限', confidential: '机密' };
export type PostInput = { project: string; type: Post['type']; classification: Post['classification']; title: string;
  body: string; summary: string; tags: string; sources: Source[] };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const hash = /^[0-9a-f]{64}$/;
const stamp = (v: unknown) => typeof v === 'string' && Number.isFinite(Date.parse(v));
const version = (v: unknown) => Number.isSafeInteger(v) && Number(v) > 0;
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
function invalid(message: string): never { throw new MobileError('INVALID_POST_INPUT', message); }
function unicode(value: string, max: number, name: string) {
  if ([...value].length > max || [...value].some(c => c.codePointAt(0)! >= 0xd800 && c.codePointAt(0)! <= 0xdfff)) invalid(name + '超过长度限制或包含无效字符。');
  return value;
}
export function postInput(post?: Post, project?: Schemas['Project']): PostInput {
  return { project: post?.project_id ?? project?.id ?? '', type: post?.type ?? 'project_log',
    classification: post?.classification ?? project?.classification ?? 'internal', title: post?.title ?? '', body: post?.body ?? '',
    summary: post?.summary ?? '', tags: post?.tags.join('，') ?? '', sources: (post?.source_refs ?? []).map(s => ({ ...s })) };
}
export function postSources(input: Source[]): Source[] {
  if (!Array.isArray(input) || input.length > 100) invalid('知识附件最多100项。');
  return input.map(s => {
    if (s.type !== 'file' || !uuid.test(s.id) || s.version_id != null && !uuid.test(s.version_id)) invalid('附件须为实际文件及明确的固定版本或跟随当前版。');
    return { type: 'file', id: s.id, ...(s.version_id ? { version_id: s.version_id } : {}) };
  });
}
// Same field order, names, HTML escapes and omitted nil version as Go ContentHash.
// A mismatch prevents replacing a filtered attachment set with a partial list.
export function postHash(post: Pick<Post, 'project_id' | 'type' | 'classification' | 'current_revision_id' | 'title' | 'body' | 'summary' | 'tags' | 'source_refs'>): string {
  const json = JSON.stringify({ project_id: post.project_id, Type: post.type, Classification: post.classification,
    RevisionID: post.current_revision_id, Title: post.title, Body: post.body, Summary: post.summary, Tags: post.tags, Sources: postSources(post.source_refs) })
    .replace(/[<>&\u2028\u2029]/g, value => '\\u' + value.charCodeAt(0).toString(16).padStart(4, '0'));
  return bytesToHex(sha256(utf8ToBytes(json)));
}
export function completePost(post: Post): boolean {
  try { return hash.test(post.content_hash) && postHash(post) === post.content_hash; } catch { return false; }
}
export function postPayload(input: PostInput, before?: Post): Schemas['PostCreate'] | Schemas['PostPatch'] {
  if (!Object.prototype.hasOwnProperty.call(postTypes, input.type) || !Object.prototype.hasOwnProperty.call(postClasses, input.classification)
    || input.project && !uuid.test(input.project)) invalid('请核对知识类型、当前可见项目与资料分类。');
  const title = unicode(input.title.trim(), 200, '标题'); if (!title) invalid('请填写知识标题。');
  const tags = before && input.tags === before.tags.join('，') ? [...before.tags] : input.tags.trim() ? input.tags.split(/[,，]/).map(s => unicode(s.trim(), 50, '标签')) : [];
  if (tags.length > 20 || tags.some(s => !s) || new Set(tags.map(s => s.toLowerCase())).size !== tags.length) invalid('标签最多20项，每项1—50字符；不能留空或重复。');
  const value = { project_id: input.project || null, type: input.type, classification: input.classification,
    title, body: unicode(input.body, 20000, '正文'), summary: unicode(input.summary, 20000, '摘要'), tags, source_refs: postSources(input.sources) };
  if (!before) return value;
  if (!before.can_edit || before.state === 'archived' || !completePost(before) || !uuid.test(before.id) || !uuid.test(before.current_revision_id) || !version(before.version)) invalid('当前权限、项目阶段或完整修订尚不能编辑；请重新读取。');
  if (value.project_id !== before.project_id || value.type !== before.type || value.classification !== before.classification) invalid('项目范围、类型与分类不能通过正文编辑更换。');
  return { ...value, expected_version: before.version, base_revision_id: before.current_revision_id };
}
function sameSources(a: Source[], b: Source[]): boolean { return JSON.stringify(postSources(a)) === JSON.stringify(postSources(b)); }
function sameBody(a: Pick<Post, 'title' | 'body' | 'summary' | 'tags' | 'source_refs'>, b: Pick<Post, 'title' | 'body' | 'summary' | 'tags' | 'source_refs'>): boolean {
  return a.title === b.title && a.body === b.body && a.summary === b.summary && JSON.stringify(a.tags) === JSON.stringify(b.tags) && sameSources(a.source_refs, b.source_refs);
}
export function postReceipt(value: unknown, input: PostInput, authorID: string, before?: Post): value is Post {
  if (!validPost(value)) return false;
  try {
    const body = postPayload(input, before);
    return value.version === (before ? before.version + 1 : 1) && (!before || value.id === before.id)
      && value.author_id === (before?.author_id ?? authorID) && value.project_id === body.project_id && value.type === body.type
      && value.classification === body.classification && value.state === (before?.state ?? 'draft')
      && value.published_revision_id === (before?.published_revision_id ?? null) && value.current_revision_id !== before?.current_revision_id
      && value.has_unpublished_changes && sameBody(value, body as Post) && completePost(value);
  } catch { return false; }
}
export function validPost(value: unknown): value is Post {
  return record(value) && typeof value.id === 'string' && uuid.test(value.id) && typeof value.current_revision_id === 'string' && uuid.test(value.current_revision_id)
    && Object.prototype.hasOwnProperty.call(postTypes, String(value.type)) && Object.prototype.hasOwnProperty.call(postClasses, String(value.classification))
    && version(value.version) && stamp(value.created_at) && stamp(value.updated_at) && typeof value.author_id === 'string' && uuid.test(value.author_id)
    && typeof value.title === 'string' && typeof value.body === 'string' && typeof value.summary === 'string' && Array.isArray(value.tags)
    && value.tags.every(v => typeof v === 'string') && Array.isArray(value.source_refs) && typeof value.content_hash === 'string' && hash.test(value.content_hash)
    && ['draft', 'published', 'archived'].includes(String(value.state)) && (value.project_id === null || typeof value.project_id === 'string' && uuid.test(value.project_id))
    && (value.published_revision_id === null || typeof value.published_revision_id === 'string' && uuid.test(value.published_revision_id))
    && typeof value.has_unpublished_changes === 'boolean' && typeof value.can_edit === 'boolean' && typeof value.can_publish === 'boolean'
    && typeof value.can_read_history === 'boolean' && typeof value.can_comment === 'boolean';
}
export function postReason(value: string): string { const result = unicode(value.trim(), 200, '原因'); if (!result) invalid('请明确填写本次操作原因。'); return result; }
export function transitionPayload(post: Post, action: Schemas['PostTransition']['action'], reason: string, checked: boolean): Schemas['PostTransition'] {
  if (!['publish', 'archive', 'draft'].includes(action) || !post.can_publish || !checked || action === 'publish' && (!completePost(post) || !post.body.trim() || post.state === 'archived')
    || action !== 'publish' && !['published', 'archived'].includes(post.state) || action === 'archive' && post.state !== 'published') invalid('请重新核对当前修订、状态、全部内容与本人操作。');
  return { expected_version: post.version, base_revision_id: post.current_revision_id, action, reason: postReason(reason) };
}
export function transitionReceipt(value: unknown, before: Post, action: Schemas['PostTransition']['action']): value is Post {
  if (!validPost(value)) return false;
  return value.id === before.id && value.version === before.version + 1 && value.current_revision_id === before.current_revision_id
    && value.project_id === before.project_id && value.type === before.type && value.classification === before.classification && value.author_id === before.author_id
    && value.content_hash === before.content_hash && sameBody(value, before) && value.state === ({ publish: 'published', archive: 'archived', draft: 'draft' } as const)[action]
    && value.published_revision_id === (action === 'publish' ? before.current_revision_id : before.published_revision_id)
    && value.has_unpublished_changes === (action === 'publish' ? false : before.has_unpublished_changes);
}
export function restoreReceipt(value: unknown, before: Post, revision: Revision): value is Post {
  if (!validPost(value)) return false;
  return revision.post_id === before.id && value.id === before.id && value.version === before.version + 1 && value.current_revision_id !== before.current_revision_id
    && value.current_revision_id !== revision.id && value.project_id === before.project_id && value.type === before.type && value.classification === before.classification
    && value.author_id === before.author_id && value.state === before.state && value.published_revision_id === before.published_revision_id
    && value.has_unpublished_changes && sameBody(value, revision) && completePost(value);
}
export function revisionReceipt(value: Revision, post: Post, before?: Post): boolean {
  return value.id === post.current_revision_id && value.post_id === post.id && version(value.seq) && stamp(value.created_at)
    && (!before || value.base_revision_id === before.current_revision_id) && sameBody(value, post);
}
export function commentPayload(post: Post, body: string, attachments: string[] | undefined, comment?: Schemas['Comment']): Schemas['PostCommentCreate'] | Schemas['CommentPatch'] {
  if (!post.can_comment || !body.trim()) invalid('请核对当前知识修订及评论权限并填写文字。');
  unicode(body, 20000, '评论');
  if (attachments && (attachments.length > 20 || new Set(attachments).size !== attachments.length || attachments.some(id => !uuid.test(id)))) invalid('评论最多20个不同的固定版本。');
  if (comment && (comment.resource_id !== post.id || comment.resource_type !== 'post' || !version(comment.revision))) invalid('评论不属于当前知识资料。');
  return { body, expected_context_revision_id: post.current_revision_id, ...(comment ? { expected_revision: comment.revision } : {}),
    ...(attachments === undefined ? {} : { attachment_versions: [...attachments] }) };
}
export function commentReceipt(value: unknown, post: Post, authorID: string, body: string, attachments: string[], before?: Schemas['Comment']): value is Schemas['Comment'] {
  return record(value) && typeof value.id === 'string' && uuid.test(value.id) && stamp(value.created_at) && value.resource_type === 'post'
    && value.resource_id === post.id && value.author_id === authorID && value.body === body && value.revision === (before ? before.revision + 1 : 1)
    && value.context_revision_id === (before?.context_revision_id ?? post.current_revision_id) && value.replaces_id === (before?.id ?? null)
    && (!before || value.id !== before.id) && Array.isArray(value.attachment_versions) && JSON.stringify(value.attachment_versions) === JSON.stringify(attachments);
}
