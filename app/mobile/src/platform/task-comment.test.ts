import test from 'node:test';
import assert from 'node:assert/strict';
import { taskCommentPayload, taskCommentReceipt, type TaskComment } from '../../../packages/contracts/src/task-comment';
import { notificationDestination } from '../../../packages/contracts/src/notifications';
const task = '20000000-0000-4000-8000-000000000001', author = '20000000-0000-4000-8000-000000000002', member = '20000000-0000-4000-8000-000000000003', comment = '20000000-0000-4000-8000-000000000004', project = '20000000-0000-4000-8000-000000000005', file = '20000000-0000-4000-8000-000000000006';
const first: TaskComment = { id: comment, resource_type: 'task', resource_id: task, author_id: author, author_display_name: 'Alice', body: 'Review', revision: 1, attachment_versions: [file], mentioned_user_ids: [member], mentions_filtered: false, replaces_id: null, context_revision_id: null, created_at: '2026-10-28T01:00:00Z' };
test('task comments preserve hidden original associations on text-only edits', () => {
  const input = taskCommentPayload(task, 'Edited', undefined, undefined, { ...first, mentioned_user_ids: [], mentions_filtered: true });
  assert.deepEqual(input, { body: 'Edited', expected_revision: 1 });
  assert.throws(() => taskCommentPayload(task, 'Edited', [], [], { ...first, mentions_filtered: true }));
  assert.deepEqual(taskCommentPayload(task, 'Edited', [], [], first), { body: 'Edited', expected_revision: 1, attachment_versions: [], mentioned_user_ids: [] });
});
test('task comment Unicode and unique-identity bounds match the server', () => {
  assert.equal(taskCommentPayload(task, '😀'.repeat(20000), [], []).body.length, 40000);
  assert.throws(() => taskCommentPayload(task, '😀'.repeat(20001), [], []));
  assert.throws(() => taskCommentPayload(task, '  ', [], []));
  assert.throws(() => taskCommentPayload(task, 'Review', [file, file], []));
  assert.throws(() => taskCommentPayload(task, 'Review', [], [member, member]));
  assert.throws(() => taskCommentPayload(task, 'Review', [], ['../member']));
  assert.throws(() => taskCommentPayload(task, 'Review', [], [], { ...first, resource_type: 'post' }));
});
test('task comment receipts require exact current revision and explicit mention fields', () => {
  const input = taskCommentPayload(task, first.body, [file], [member]);
  assert.equal(taskCommentReceipt(first, task, author, input), true);
  for (const patch of [{ mentioned_user_ids: undefined }, { mentions_filtered: undefined }, { mentions_filtered: true }, { mentioned_user_ids: [] }, { attachment_versions: [] }, { body: 'Other' }, { resource_id: project }, { author_id: member }, { context_revision_id: project }, { revision: 2 }]) assert.equal(taskCommentReceipt({ ...first, ...patch }, task, author, input), false);
});
test('task comment edits pin the immutable predecessor and fixed associations', () => {
  const input = taskCommentPayload(task, 'Edited', undefined, undefined, first);
  const second = { ...first, id: project, revision: 2, replaces_id: comment, body: 'Edited' };
  assert.equal(taskCommentReceipt(second, task, author, input, first), true);
  assert.equal(taskCommentReceipt({ ...second, replaces_id: null }, task, author, input, first), false);
  assert.equal(taskCommentReceipt({ ...second, attachment_versions: [] }, task, author, input, first), false);
});
test('task mention notifications allow only exact canonical comment routes', () => {
  const target = { type: 'task' as const, id: task, container_id: project, comment_id: comment, web_path: '/projects/' + project + '?task=' + task + '&comment=' + comment };
  assert.equal(notificationDestination(target).mobile, '/pages/detail/index?kind=task&id=' + task + '&comment_id=' + comment);
  assert.throws(() => notificationDestination({ ...target, comment_id: '../escape' }));
  assert.throws(() => notificationDestination({ ...target, web_path: 'https://example.test' }));
  assert.throws(() => notificationDestination({ ...target, type: 'request' }));
});
