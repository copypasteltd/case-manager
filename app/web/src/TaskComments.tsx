import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { taskCommentPayload, taskCommentReceipt, type TaskComment } from '@copypaste/contracts/task-comment';
import { api, ApiError, type Me, type Page, type Project, type Schemas, type Task } from './api';
import { Busy, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { FileVersionPicker, type FileReference } from './BusinessSelectors';
import { DownloadDialog } from './DownloadDialog';
import Markdown from './Markdown';

type Candidate = Schemas['MentionCandidate'];
const textDrafts = new Map<string, { body: string; at: number }>();
const intents = new Map<string, { signature: string; key: string }>();
let authority = '';
if (typeof window !== 'undefined') window.addEventListener('cp:session-expired', () => { authority = ''; textDrafts.clear(); intents.clear(); });

export default function TaskComments({ task, project, me, readonly }: { task: Task; project: Project; me: Me; readonly: boolean }) {
  const focusRef = useRef<HTMLElement>(null);
  const cache = useQueryClient(), [params] = useSearchParams(), focusID = params.get('comment') ?? '';
  const binding = me.user.id + ':' + me.authorization_version + ':' + me.session_expires_at;
  if (authority !== binding) { authority = binding; textDrafts.clear(); intents.clear(); }
  const [creating, setCreating] = useState(false), [editing, setEditing] = useState<TaskComment>(), [history, setHistory] = useState<TaskComment>();
  const list = useInfiniteQuery({ queryKey: ['task-comments', task.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<TaskComment>>('/tasks/' + task.id + '/comments?limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const members = useQuery({ queryKey: ['project-members', task.project_id], queryFn: ({ signal }) => api<Schemas['ProjectMembers']>('/projects/' + task.project_id + '/members', { signal }) });
  const validFocus = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(focusID);
  const focus = useQuery({ queryKey: ['task-comment-focus', focusID], enabled: validFocus, queryFn: ({ signal }) => api<TaskComment>('/comments/' + focusID, { signal }) });
  useEffect(() => {
    if (focus.data?.resource_type === 'task' && focus.data.resource_id === task.id) {
      focusRef.current?.focus({ preventScroll: true });
      focusRef.current?.scrollIntoView({ block: 'start', behavior: 'auto' });
    }
  }, [focus.data?.id, task.id]);
  const render = (c: TaskComment, exact = false) => <article ref={exact ? focusRef : undefined} tabIndex={exact ? -1 : undefined} aria-label={exact ? '通知指定讨论' : undefined} className="comment-item" key={(exact ? 'exact:' : '') + c.id} data-task-comment={c.id}>
    <div className="section-heading"><strong>{c.author_display_name}</strong><small className="muted">{date(c.created_at)} · 评论修订 {c.revision}{exact ? ' · 通知指定讨论' : ''}</small></div>
    <Markdown body={c.body} />{!!c.mentioned_user_ids?.length && <p className="muted">明确提及：{c.mentioned_user_ids.map(id => members.data?.items.find(u => u.id === id)?.display_name ?? '有权项目成员').join('、')}</p>}
    {c.mentions_filtered && <p className="muted">部分原提及当前不可查阅，文字修改将保留原记录。</p>}
    {c.attachment_versions.map(id => <FixedAttachment key={id} versionID={id} />)}
    <div className="comment-compose-actions">{!readonly && c.author_id === me.user.id && <button className="button secondary" onClick={() => { setEditing(c); setCreating(false); }}>修改我的任务评论</button>}<button className="text-button" onClick={() => setHistory(c)}>查看任务评论历史</button></div>
  </article>;
  function done() { setCreating(false); setEditing(undefined); void cache.invalidateQueries({ queryKey: ['task-comments', task.id] }); void cache.invalidateQueries({ queryKey: ['task-comment-focus'] }); }
  return <section className="section-block" aria-label="任务讨论与提及"><div className="section-heading"><h3>任务讨论与提及</h3>{!readonly && <button className="button secondary" onClick={() => { setCreating(true); setEditing(undefined); }}>新增任务评论</button>}</div>
    <p className="muted">明确选择有权项目成员才发送站内提醒。正文中的 @文字不会自动通知；文字修订默认保留原固定附件和提及。</p>
    <ErrorNotice error={list.error ?? members.error ?? focus.error} />
    {focusID && !validFocus && <p className="notice warning">讨论地址无效，请从通知中心重新打开。</p>}
    {validFocus && focus.isPending && <Busy label="正在读取通知指定讨论" />}
    {focus.data && (focus.data.resource_type === 'task' && focus.data.resource_id === task.id ? render(focus.data, true) : <p className="notice danger">指定讨论与当前任务不一致。</p>)}
    {list.isPending ? <Busy label="正在读取任务讨论" /> : list.data?.pages.flatMap(p => p.items).map(c => render(c))}
    {!list.isPending && !list.error && !list.data?.pages[0]?.items.length && <Empty title="此任务暂无讨论" detail="可新增评论、选择固定附件并明确提及成员。" />}
    {list.hasNextPage && <button className="button secondary" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>更多任务讨论</button>}
    <button className="text-button" onClick={() => void list.refetch()}>重新读取任务讨论</button>
    {(creating || editing) && <TaskCommentEditor key={binding + ':' + (editing?.id ?? 'new')} binding={binding + ':' + task.id + ':' + (editing?.id ?? 'new')} task={task} project={project} me={me} before={editing} onDone={done} onClose={() => { setCreating(false); setEditing(undefined); }} />}
    {history && <History comment={history} onClose={() => setHistory(undefined)} />}
  </section>;
}

function FixedAttachment({ versionID }: { versionID: string }) {
  const [reading, setReading] = useState(false);
  const version = useQuery({ queryKey: ['comment-fixed-file', versionID], queryFn: ({ signal }) => api<Schemas['FileVersion']>('/file-versions/' + versionID, { signal }) });
  return <div className="conversation-source-preview"><ErrorNotice error={version.error} />{version.isPending ? <Busy label="正在核对固定附件" /> : version.data?.id === versionID && <><strong>{version.data.original_name}</strong><p className="muted">固定版本 {version.data.seq} · SHA256 <span style={{ overflowWrap: 'anywhere' }}>{version.data.sha256}</span></p><button className="button secondary" onClick={() => setReading(true)}>查阅此固定版本</button>{reading && <DownloadDialog version={version.data} onClose={() => setReading(false)} />}</>}</div>;
}

function TaskCommentEditor({ task, project, me, before, binding, onDone, onClose }: { task: Task; project: Project; me: Me; before?: TaskComment; binding: string; onDone(): void; onClose(): void }) {
  const [initial] = useState(() => textDrafts.get(binding)), restored = Boolean(initial && Date.now() - initial.at < 15 * 60000);
  const [body, setBody] = useState(restored ? initial!.body : before?.body ?? ''), [files, setFiles] = useState<FileReference[]>([]), [picker, setPicker] = useState(false);
  const [replaceFiles, setReplaceFiles] = useState(false), [replaceMentions, setReplaceMentions] = useState(false), [selected, setSelected] = useState<Candidate[]>([]);
  const [query, setQuery] = useState(''), [search, setSearch] = useState(''), [preview, setPreview] = useState<ReturnType<typeof taskCommentPayload>>(), [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false), [blocked, setBlocked] = useState(false), [error, setError] = useState<unknown>();
  const pending = useRef(false), alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const candidates = useInfiniteQuery({ queryKey: ['task-mention-candidates', task.id, query], initialPageParam: '', enabled: !before || replaceMentions,
    queryFn: ({ pageParam, signal }) => api<Page<Candidate>>('/tasks/' + task.id + '/mention-candidates?limit=20&q=' + encodeURIComponent(query) + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: p => p.next_cursor ?? undefined });
  const disabled = busy || blocked;
  function reset() { setPreview(undefined); setConfirmed(false); }
  function changeBody(value: string) {
    setBody(value); reset();
    for (const [key, draft] of textDrafts) if (Date.now() - draft.at >= 15 * 60000) textDrafts.delete(key);
    if (textDrafts.size >= 32 && !textDrafts.has(binding)) textDrafts.delete(textDrafts.keys().next().value!);
    textDrafts.set(binding, { body: value, at: Date.now() });
    while ([...textDrafts.values()].reduce((sum, entry) => sum + entry.body.length, 0) > 100000 && textDrafts.size > 1) textDrafts.delete([...textDrafts.keys()].find(key => key !== binding)!);
  }
  async function save() {
    if (!preview || !confirmed || pending.current || disabled) return;
    pending.current = true; setBusy(true); setError(undefined);
    let received = false, key = '';
    try {
      const signature = JSON.stringify({ path: before ? '/comments/' + before.id : '/tasks/' + task.id + '/comments', input: preview });
      const prior = intents.get(binding);
      if (prior && prior.signature !== signature) throw new ApiError(409, 'PREVIOUS_RESULT_UNCONFIRMED', '上次任务评论结果尚未核对，请先查看当前讨论；不能改写为另一次创建。', '');
      if (!prior && intents.size >= 128) throw new Error('本次会话有过多待核对操作，请先核对已有讨论。');
      key = prior?.key ?? crypto.randomUUID(); intents.set(binding, { signature, key });
      const value = await api<TaskComment>(before ? '/comments/' + before.id : '/tasks/' + task.id + '/comments', { method: before ? 'PATCH' : 'POST', body: preview, key });
      received = true;
      if (!taskCommentReceipt(value, task.id, me.user.id, preview, before)) throw new Error('评论回执不完整，请先重新读取任务讨论核对结果。');
      const exact = await api<TaskComment>('/comments/' + value.id);
      const history = await api<Page<TaskComment>>('/comments/' + value.id + '/history?limit=20');
      if (!taskCommentReceipt(exact, task.id, me.user.id, preview, before) || !history.items.some(c => c.id === value.id && taskCommentReceipt(c, task.id, me.user.id, preview, before)) || before && !history.items.some(c => c.id === before.id && c.body === before.body && c.revision === before.revision)) throw new Error('评论保存结果与当前记录尚未核对，请先刷新讨论。');
      if (alive.current) { intents.delete(binding); textDrafts.delete(binding); setBlocked(true); onDone(); }
    } catch (cause) {
      const uncertain = received || cause instanceof ApiError && ['RESULT_UNCONFIRMED', 'INVALID_RESPONSE', 'PREVIOUS_RESULT_UNCONFIRMED'].includes(cause.code);
      if (!uncertain && key && intents.get(binding)?.key === key) intents.delete(binding);
      if (alive.current) { setError(cause); if (uncertain || cause instanceof ApiError && [403, 404, 409, 412].includes(cause.status)) setBlocked(true); }
    } finally { pending.current = false; if (alive.current) setBusy(false); }
  }
  function prepare(event: FormEvent) { event.preventDefault(); try { setError(undefined); setPreview(taskCommentPayload(task.id, body, before && !replaceFiles ? undefined : files.map(f => f.version_id), before && !replaceMentions ? undefined : selected.map(c => c.id), before)); setConfirmed(false); } catch (cause) { setError(cause); } }
  return <Dialog title={before ? '修改任务评论 · 新修订' : '新增任务评论'} busy={busy} onClose={onClose}><form onSubmit={prepare}>
    {restored && <p className="notice">已恢复此会话内未提交的文字。固定附件、成员选择及核对声明需要重新操作。</p>}
    <Field label="任务评论文字" hint="1至20000字符；正文中的 @文字不会自动发送通知。"><textarea aria-label="任务评论文字" rows={6} maxLength={40000} disabled={disabled} value={body} onChange={e => changeBody(e.target.value)} /></Field>
    {before && <><label className="check-field"><input type="checkbox" checked={replaceFiles} disabled={disabled} onChange={e => { setReplaceFiles(e.target.checked); setFiles([]); setPicker(false); reset(); }} />明确替换全部原附件（不选新附件表示清空）</label>
      <label className="check-field"><input type="checkbox" checked={replaceMentions} disabled={disabled || before.mentions_filtered} onChange={e => { setReplaceMentions(e.target.checked); setSelected([]); reset(); }} />明确替换全部原提及（不选成员表示清空）</label><p className="muted">默认只修改文字并保留全部原关联。部分原资料不可核对时，服务器会拒绝显式替换。</p></>}
    {(!before || replaceFiles) && <><button type="button" className="button secondary" disabled={disabled || files.length >= 20} onClick={() => setPicker(true)}>选择任务评论固定附件</button>{files.map(f => <div className="conversation-source-preview" key={f.version_id}>{f.name} · 固定版本 {f.seq}<button type="button" className="text-button" disabled={disabled} onClick={() => { setFiles(old => old.filter(v => v.version_id !== f.version_id)); reset(); }}>移除此附件选择</button></div>)}</>}
    {(!before || replaceMentions) && <fieldset className="form-fieldset" disabled={disabled}><legend>明确选择提及成员</legend><Field label="搜索有权项目成员"><input aria-label="搜索有权项目成员" maxLength={200} value={search} onChange={e => setSearch(e.target.value)} /></Field><button type="button" className="button secondary" onClick={() => setQuery(search.trim())}>搜索提及成员</button>
      <p className="muted">已选 {selected.length}/20：{selected.map(c => c.display_name).join('、') || '无'}。仅新增提及产生提醒。</p><ErrorNotice error={candidates.error} />
      {candidates.isPending ? <Busy label="正在读取有权成员" /> : candidates.data?.pages.flatMap(p => p.items).map(c => <label className="check-field" key={c.id}><input type="checkbox" checked={selected.some(v => v.id === c.id)} disabled={selected.length >= 20 && !selected.some(v => v.id === c.id)} onChange={e => { setSelected(old => e.target.checked ? [...old.filter(v => v.id !== c.id), c] : old.filter(v => v.id !== c.id)); reset(); }} />{c.display_name}</label>)}
      {!candidates.isPending && !candidates.error && !candidates.data?.pages[0]?.items.length && <p className="muted">没有可选的其他有权成员。</p>}
      {candidates.hasNextPage && <button type="button" className="button secondary" disabled={candidates.isFetchingNextPage} onClick={() => void candidates.fetchNextPage()}>更多提及成员</button>}
      {!!selected.length && <button type="button" className="text-button" onClick={() => { setSelected([]); reset(); }}>清空本次提及选择</button>}
    </fieldset>}
    <ErrorNotice error={error} />{blocked && <p className="notice warning" role="status">本次操作需要核对。文字仍保留，请先关闭表单并重新读取任务讨论，避免重复提交。</p>}
    <button type="submit" className="button secondary" disabled={disabled}>核对任务评论</button>
    {preview && <section className="conversation-source-preview" aria-label="核对本次任务评论"><h3>核对本次任务评论</h3><Markdown body={preview.body} /><p className="muted">固定附件：{preview.attachment_versions === undefined ? '保留全部原关联' : preview.attachment_versions.length + ' 个'}；提及：{preview.mentioned_user_ids === undefined ? '保留全部原关联' : selected.map(c => c.display_name).join('、') || '无'}。</p>
      <label className="check-field"><input type="checkbox" checked={confirmed} disabled={disabled} onChange={e => setConfirmed(e.target.checked)} />已核对文字、固定附件及提及范围</label><button type="button" className="button primary" disabled={disabled || !confirmed} onClick={() => void save()}>{busy ? '正在保存并核对…' : before ? '保存任务评论新修订' : '保存任务评论'}</button></section>}
  </form>{picker && <FileVersionPicker project={task.project_id} classification={project.classification ?? 'project_restricted'} title="选择任务评论固定附件" onClose={() => setPicker(false)} onSelect={value => { setFiles(old => [...old.filter(v => v.version_id !== value.version_id), value]); setPicker(false); reset(); }} />}</Dialog>;
}

function History({ comment, onClose }: { comment: TaskComment; onClose(): void }) {
  const history = useInfiniteQuery({ queryKey: ['task-comment-history', comment.id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<TaskComment>>('/comments/' + comment.id + '/history?limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: p => p.next_cursor ?? undefined });
  return <Dialog title="任务评论不可变历史" onClose={onClose}><ErrorNotice error={history.error} />{history.isPending ? <Busy label="正在读取评论历史" /> : history.data?.pages.flatMap(p => p.items).map(c => <article key={c.id} className="comment-item"><h3>评论修订 {c.revision}</h3><small>{date(c.created_at)}</small><Markdown body={c.body} /><p className="muted">当前可见提及 {c.mentioned_user_ids?.length ?? 0} 人{c.mentions_filtered ? '；部分原成员已过滤' : ''}</p>{c.attachment_versions.map(id => <FixedAttachment key={id} versionID={id} />)}</article>)}{history.hasNextPage && <button className="button secondary" disabled={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>更多任务评论历史</button>}</Dialog>;
}
