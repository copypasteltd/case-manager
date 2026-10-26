import { useState, useEffect, useRef, type FormEvent } from 'react';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { MessageSquare, Send, History, Pencil } from 'lucide-react';
import { api, ApiError, type Me, type Schemas } from './api';
import { Busy, date, Dialog, Empty, ErrorNotice, Field } from './components';
import Markdown from './Markdown';
type Comment = Schemas['Comment'];

export default function Comments({ type, id, revisionID = '', me, readonly }: { type: 'post' | 'task'; id: string; revisionID?: string; me: Me; readonly: boolean }) {
  const cache = useQueryClient(); const [body, setBody] = useState(''), [editing, setEditing] = useState<Comment | null>(null), [history, setHistory] = useState<Comment | null>(null);
  const previousContext = useRef(revisionID), [contextChanged, setContextChanged] = useState(false);
  useEffect(() => {
    if (previousContext.current !== revisionID) {
      previousContext.current = revisionID;
      if (body.trim() || editing) setContextChanged(true);
    }
  }, [revisionID, body, editing]);
  const key = ['comments', type, id, revisionID];
  const query = useInfiniteQuery({
    queryKey: key, initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Schemas['Comments']>('/' + (type === 'post' ? 'posts' : 'tasks') + '/' + id + '/comments?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined,
  });
  const mutation = useMutation({
    mutationFn: () => api<Comment>(editing ? '/comments/' + editing.id : '/' + (type === 'post' ? 'posts' : 'tasks') + '/' + id + '/comments', { method: editing ? 'PATCH' : 'POST', body: { body, ...(type === 'post' ? { expected_context_revision_id: editing?.context_revision_id ?? revisionID } : {}), ...(editing ? { expected_revision: editing.revision, attachment_versions: editing.attachment_versions } : {}) } }),
    onSuccess: async () => { setBody(''); setEditing(null); await cache.invalidateQueries({ queryKey: ['comments', type, id] }); },
    onError: error => { if (error instanceof ApiError && error.code === 'COMMENT_CONTEXT_CHANGED') void cache.invalidateQueries({ queryKey: ['post', id] }); },
  });
  function submit(event: FormEvent<HTMLFormElement>) { event.preventDefault(); mutation.mutate(); }
  const items = query.data?.pages.flatMap(page => page.items) ?? [];
  return <section className="comments-section"><div className="section-heading compact"><h2><MessageSquare size={19} />讨论与记录</h2>{type === 'post' && <small className="muted">评论关联当前可见修订</small>}</div><ErrorNotice error={query.error} />
    {!readonly && me.actions.includes('comment.write') && <form className="comment-compose" onSubmit={submit}><ErrorNotice error={mutation.error} />{contextChanged && <div className="notice neutral" role="status"><div><strong>正文已更新，评论内容已保留。</strong><p>核对当前正文后，可以将保留内容作为这次修订的新评论。</p><button type="button" className="text-button" onClick={() => { setEditing(null); setContextChanged(false); mutation.reset(); }}>已核对当前正文</button></div></div>}<Field label={editing ? '修改我的评论' : '写一条评论'} hint="支持基础 Markdown；修改会保留原文记录。"><textarea value={body} onChange={event => setBody(event.target.value)} required maxLength={20000} rows={3} placeholder="记录进展、补充依据，或留下需要一起确认的事。" /></Field><div className="comment-compose-actions">{editing && <button type="button" className="button secondary" disabled={mutation.isPending} onClick={() => { setEditing(null); setBody(''); setContextChanged(false); }}>取消修改</button>}<button className="button primary" disabled={mutation.isPending || contextChanged || !body.trim()}><Send size={15} />{mutation.isPending ? '正在保存…' : editing ? '保存评论修订' : '发布评论'}</button></div></form>}
    {query.isPending ? <Busy label="正在读取讨论" /> : !items.length && !query.error ? <Empty title="还没有讨论记录" detail="把需要一起确认的事，留在资料与任务旁边。" /> : <div className="comment-list">{items.map(comment => <article className="comment-item" key={comment.id}><header><span className="avatar small">{comment.author_display_name.slice(0, 1)}</span><strong>{comment.author_display_name}</strong><small>{date(comment.created_at)} · 修订 {comment.revision}</small><div>{comment.revision > 1 && <button className="text-button" onClick={() => setHistory(comment)}><History size={14} />修改记录</button>}{!readonly && me.actions.includes('comment.write') && comment.author_id === me.user.id && <button className="text-button" onClick={() => { setEditing(comment); setBody(comment.body); mutation.reset(); }}><Pencil size={14} />编辑</button>}</div></header><Markdown body={comment.body} />{comment.attachment_versions.length > 0 && <small className="muted">此评论关联 {comment.attachment_versions.length} 个固定附件版本。</small>}</article>)}</div>}
    {query.hasNextPage && <div className="list-footer"><span>已显示 {items.length} 条讨论</span><button className="button secondary" onClick={() => void query.fetchNextPage()} disabled={query.isFetchingNextPage}>加载更多讨论</button></div>}
    {history && <CommentHistory comment={history} onClose={() => setHistory(null)} />}
  </section>;
}
function CommentHistory({ comment, onClose }: { comment: Comment; onClose: () => void }) {
  const query = useInfiniteQuery({ queryKey: ['comment-history', comment.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Schemas['Comments']>('/comments/' + comment.id + '/history?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  return <Dialog title="评论修改记录" onClose={onClose}><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : query.data?.pages.flatMap(page => page.items).map(value => <article className="revision-card" key={value.id}><small className="muted">修订 {value.revision} · {date(value.created_at)}</small><Markdown body={value.body} /></article>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>更早的修订</button>}</Dialog>;
}
