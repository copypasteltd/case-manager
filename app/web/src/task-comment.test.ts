import { describe, expect, it } from 'vitest';
import { taskCommentPayload, taskCommentReceipt, type TaskComment } from '@copypaste/contracts/task-comment';
import { notificationDestination } from '@copypaste/contracts/notifications';
const task = '10000000-0000-4000-8000-000000000001', author = '10000000-0000-4000-8000-000000000002', member = '10000000-0000-4000-8000-000000000003', comment = '10000000-0000-4000-8000-000000000004', project = '10000000-0000-4000-8000-000000000005', attachment = '10000000-0000-4000-8000-000000000006';
const original: TaskComment = { id: comment, resource_type: 'task', resource_id: task, author_id: author, author_display_name: 'Alice', body: 'Review', revision: 1, attachment_versions: [attachment], mentioned_user_ids: [member], mentions_filtered: false, replaces_id: null, context_revision_id: null, created_at: '2026-10-28T01:00:00Z' };
describe('task discussion fixed input and receipts', () => {
  it('preserves omitted original associations, including hidden recipients', () => {
    const input = taskCommentPayload(task, 'Edited', undefined, undefined, { ...original, mentioned_user_ids: [], mentions_filtered: true });
    expect(input).toEqual({ body: 'Edited', expected_revision: 1 });
    expect(() => taskCommentPayload(task, 'Edited', undefined, [], { ...original, mentions_filtered: true })).toThrow('不可核对');
    expect(taskCommentPayload(task, 'Edited', [], [], original)).toEqual({ body: 'Edited', expected_revision: 1, attachment_versions: [], mentioned_user_ids: [] });
  });
  it('enforces Unicode length, unique fixed identities and task context', () => {
    expect(taskCommentPayload(task, '😀'.repeat(20000), [], []).body.length).toBe(40000);
    for (const body of ['', '  ', '😀'.repeat(20001)]) expect(() => taskCommentPayload(task, body, [], [])).toThrow();
    expect(() => taskCommentPayload(task, 'Review', [attachment, attachment], [])).toThrow();
    expect(() => taskCommentPayload(task, 'Review', [], [member, member])).toThrow();
    expect(() => taskCommentPayload(task, 'Review', [], Array.from({ length: 21 }, () => member))).toThrow();
    expect(() => taskCommentPayload(task, 'Review', [], ['https://example.test'])).toThrow();
    expect(() => taskCommentPayload(task, 'Review', [], [], { ...original, resource_type: 'post' })).toThrow();
  });
  it('requires exact creation receipts and never infers missing mention fields', () => {
    const input = taskCommentPayload(task, original.body, [attachment], [member]);
    expect(taskCommentReceipt(original, task, author, input)).toBe(true);
    for (const patch of [{ mentioned_user_ids: undefined }, { mentions_filtered: undefined }, { mentions_filtered: true }, { attachment_versions: [] }, { mentioned_user_ids: [] }, { resource_id: project }, { body: 'Different' }, { author_id: member }, { replaces_id: comment }, { context_revision_id: project }, { created_at: 'invalid' }, { revision: 2 }]) expect(taskCommentReceipt({ ...original, ...patch }, task, author, input)).toBe(false);
  });
  it('binds text edits to the selected immutable predecessor', () => {
    const input = taskCommentPayload(task, 'Edited', undefined, undefined, original);
    const revision = { ...original, id: project, body: 'Edited', revision: 2, replaces_id: comment };
    expect(taskCommentReceipt(revision, task, author, input, original)).toBe(true);
    expect(taskCommentReceipt({ ...revision, id: comment }, task, author, input, original)).toBe(false);
    expect(taskCommentReceipt({ ...revision, mentioned_user_ids: [] }, task, author, input, original)).toBe(false);
    expect(taskCommentReceipt({ ...revision, revision: 3 }, task, author, input, original)).toBe(false);
  });
  it('opens only canonical task and exact-comment destinations on both clients', () => {
    const target = { type: 'task' as const, id: task, container_id: project, comment_id: comment, web_path: '/projects/' + project + '?task=' + task + '&comment=' + comment };
    expect(notificationDestination(target)).toEqual({ web: target.web_path, mobile: '/pages/detail/index?kind=task&id=' + task + '&comment_id=' + comment });
    expect(() => notificationDestination({ ...target, comment_id: 'javascript:alert(1)' })).toThrow();
    expect(() => notificationDestination({ ...target, web_path: 'https://example.test' })).toThrow();
    expect(() => notificationDestination({ ...target, type: 'request' })).toThrow();
  });
});
