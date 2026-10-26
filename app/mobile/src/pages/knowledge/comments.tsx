import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { useCollection } from '../../platform/collection';
import { CommandNotice, useCommand } from '../../platform/command';
import { commentPayload, commentReceipt, type Post, type Source } from '../../platform/post-input';
import { client } from '../../platform/taro';
import { Scope, type Page as ResultPage, type Schemas } from '../../platform/session';
import { Action, Busy, Check, Empty, Notice, Row, Section, date } from '../../ui';
import { TextField } from '../business/controls';
import { SourceCard, SourcePicker } from './sources';

type Comment = Schemas['Comment'];
export function PostComments({ post, me }: { post: Post; me: Schemas['Me'] }) {
  const comments = useCollection<Comment>('/posts/' + post.id + '/comments?limit=20');
  const [editing, setEditing] = useState<Comment>(), [creating, setCreating] = useState(false), [history, setHistory] = useState<Comment>();
  function done() { setEditing(undefined); setCreating(false); comments.reload(); }
  return <Section title='此正文修订的讨论'><Text className='cp-muted cp-small cp-pad'>讨论绑定正在阅读的正文修订；发布前后的不同正文不会混用讨论。评论修改保留不可变历史。</Text>
    <Notice error={comments.error} />{comments.loading ? <Busy /> : comments.items.length ? comments.items.map(c => <View key={c.id} className='cp-pad'>
      <Text className='cp-row-title'>{c.author_display_name}</Text><Text className='cp-muted cp-small'>{date(c.created_at)} · 评论修订 {c.revision}</Text>
      <Text selectable className='cp-paragraph'>{c.body}</Text>{c.attachment_versions.map(id => <SourceCard key={id} versionID={id} />)}
      {post.can_comment && c.author_id === me.user.id && <Action secondary onClick={() => { setEditing(c); setCreating(false); }}>修改我的评论</Action>}
      <Action secondary onClick={() => setHistory(c)}>查看评论历史</Action>
    </View>) : !comments.error && <Empty>此正文修订暂无讨论。</Empty>}
    {comments.next && <Action secondary loading={comments.moreBusy} onClick={() => void comments.more()}>更多此修订评论</Action>}
    <Action secondary onClick={comments.reload}>重新读取当前讨论</Action>
    {post.can_comment && <Action secondary onClick={() => { setEditing(undefined); setCreating(true); }}>为此正文新增评论</Action>}
    {(creating || editing) && <CommentEditor key={editing?.id ?? 'new'} post={post} me={me} before={editing} onDone={done} onClose={() => { setEditing(undefined); setCreating(false); }} />}
    {history && <CommentHistory key={history.id} comment={history} onClose={() => setHistory(undefined)} />}
  </Section>;
}

function CommentEditor({ post, me, before, onDone, onClose }: { post: Post; me: Schemas['Me']; before?: Comment; onDone(): void; onClose(): void }) {
  const [body, setBody] = useState(before?.body ?? ''), [sources, setSources] = useState<Source[]>([]), [replace, setReplace] = useState(false), [picker, setPicker] = useState(false), [error, setError] = useState<unknown>();
  const pins = sources.map(s => s.version_id!);
  const expected = before && !replace ? before.attachment_versions : pins;
  const command = useCommand<Comment>(onDone, 'post-comment:' + post.id + ':' + post.current_revision_id + ':' + (before?.id ?? 'new'), async value => {
    if (!commentReceipt(value, post, me.user.id, body, expected, before)) return false;
    const scope = new Scope();
    try {
      const fresh = await client.request<ResultPage<Comment>>('/posts/' + post.id + '/comments?limit=20', { scope });
      const found = fresh.items.find(c => c.id === value.id);
      if (!found || !commentReceipt(found, post, me.user.id, body, expected, before)) return false;
      const history = await client.request<ResultPage<Comment>>('/comments/' + value.id + '/history?limit=20', { scope });
      return history.items.some(c => c.id === value.id && commentReceipt(c, post, me.user.id, body, expected, before))
        && (!before || history.items.some(c => c.id === before.id && c.body === before.body && c.revision === before.revision));
    } finally { scope.close(); }
  });
  const disabled = command.busy || command.blocked;
  return <View className='cp-pad'><Text className='cp-section-title'>{before ? '修改评论 · 新建评论修订' : '新增当前正文评论'}</Text>
    <TextField name='post-comment-body' label='知识评论文字' value={body} onChange={setBody} disabled={disabled} multiline max={40000} />
    {before && <><Text className='cp-muted cp-small'>文字修改默认保留服务器上的原附件，即使部分附件当前不可见。</Text>
      <Check checked={replace} onChange={v => { setReplace(v); setSources([]); setPicker(false); }} disabled={disabled}>明确替换全部原附件，以本次重新选择的版本为准（空列表表示清空）</Check></>}
    {(!before || replace) && <><Action secondary disabled={disabled} onClick={() => setPicker(!picker)}>{picker ? '收起评论附件选择' : '选择评论固定附件'}</Action>
      {replace && <Text className='cp-notice'>此操作替换全部附件。若原附件当前无权核对，服务器会拒绝替换；可取消替换，仅修改文字。</Text>}
      {picker && <SourcePicker projectID={post.project_id ?? ''} value={sources} onChange={setSources} disabled={disabled} fixedOnly max={20} />}
      {!picker && sources.map(s => <SourceCard key={s.version_id} source={s} />)}</>}
    <Notice error={error ?? command.error} /><CommandNotice blocked={command.blocked} />
    <Action loading={command.busy} disabled={disabled} onClick={() => { try { setError(undefined); const value = commentPayload(post, body, before && !replace ? undefined : pins, before); void command.submit(before ? '/comments/' + before.id : '/posts/' + post.id + '/comments', value, before ? 'PATCH' : 'POST'); } catch (cause) { setError(cause); } }}>{before ? '保存评论新修订' : '保存此正文评论'}</Action>
    <Action secondary disabled={command.busy} onClick={onClose}>关闭评论表单并核对讨论</Action>
  </View>;
}

function CommentHistory({ comment, onClose }: { comment: Comment; onClose(): void }) {
  const history = useCollection<Comment>('/comments/' + comment.id + '/history?limit=20'), [selected, setSelected] = useState<Comment>();
  return <Section title='评论不可变历史'><Notice error={history.error} />{history.loading ? <Busy /> : history.items.map(c => <Row key={c.id} title={'评论修订 ' + c.revision} subtitle={date(c.created_at) + ' · ' + c.author_display_name} onClick={() => setSelected(c)} />)}
    {history.next && <Action secondary loading={history.moreBusy} onClick={() => void history.more()}>更多评论历史</Action>}
    {selected && <View className='cp-pad'><Text selectable>{selected.body}</Text><Text className='cp-muted cp-small'>正文上下文 {selected.context_revision_id}</Text>{selected.attachment_versions.map(id => <SourceCard key={id} versionID={id} />)}</View>}
    <Action secondary onClick={onClose}>收起评论历史</Action>
  </Section>;
}
