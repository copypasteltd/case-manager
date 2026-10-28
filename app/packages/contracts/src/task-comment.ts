import type { components } from './api';
type Schemas = components['schemas'];
export type TaskComment = Schemas['Comment'];
export type TaskCommentInput = Schemas['CommentCreate'] & { expected_revision?: number };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
function ids(values: readonly string[], label: string) {
  if (values.length > 20 || values.some(value => !uuid.test(value)) || new Set(values).size !== values.length) throw new Error(label + '无效，请重新选择（最多20项）。');
  return [...values];
}
export function taskCommentPayload(taskID: string, body: string, attachments: string[] | undefined, mentions: string[] | undefined, before?: TaskComment): TaskCommentInput {
  if (!uuid.test(taskID) || before && (before.resource_type !== 'task' || before.resource_id !== taskID || !uuid.test(before.id) || !Number.isSafeInteger(before.revision) || before.revision < 1 || before.revision >= Number.MAX_SAFE_INTEGER)) throw new Error('任务讨论上下文已变化，请重新读取。');
  if (!body.trim() || body.length > 40000 || [...body].length > 20000) throw new Error('任务评论需为1至20000字符。');
  if (mentions !== undefined && before?.mentions_filtered) throw new Error('部分原提及成员当前不可核对，请保留原提及，仅修改文字。');
  return { body, ...(attachments !== undefined ? { attachment_versions: ids(attachments, '固定附件') } : before ? {} : { attachment_versions: [] }),
    ...(mentions !== undefined ? { mentioned_user_ids: ids(mentions, '提及成员') } : before ? {} : { mentioned_user_ids: [] }),
    ...(before ? { expected_revision: before.revision } : {}) };
}
function same(actual: unknown, expected: readonly string[]) {
  return Array.isArray(actual) && actual.every(value => typeof value === 'string' && uuid.test(value)) && new Set(actual).size === actual.length && actual.length === expected.length && expected.every(value => actual.includes(value));
}
// Both clients check the exact committed revision and its fixed associations.
// A missing field is an unresolved receipt, never an inferred success.
export function taskCommentReceipt(value: TaskComment, taskID: string, authorID: string, input: TaskCommentInput, before?: TaskComment): boolean {
  return Boolean(value && uuid.test(value.id) && value.resource_type === 'task' && value.resource_id === taskID && value.author_id === authorID
    && typeof value.author_display_name === 'string' && value.author_display_name.trim() && value.body === input.body
    && value.revision === (before ? before.revision + 1 : 1) && value.replaces_id === (before?.id ?? null)
    && (!before || value.id !== before.id) && value.context_revision_id === null
    && typeof value.created_at === 'string' && Number.isFinite(Date.parse(value.created_at))
    && same(value.attachment_versions, input.attachment_versions ?? before?.attachment_versions ?? [])
    && same(value.mentioned_user_ids, input.mentioned_user_ids ?? before?.mentioned_user_ids ?? [])
    && value.mentions_filtered === (input.mentioned_user_ids === undefined ? before?.mentions_filtered ?? false : false));
}
