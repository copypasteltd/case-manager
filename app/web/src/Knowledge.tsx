import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowLeft, ArrowRight, ArrowUpRight, FileText, Plus, Search, Pencil, History, Check, Archive, RotateCcw } from 'lucide-react';
import { api, type Me, type Page, type Project, type Schemas } from './api';
import { Busy, classificationNames, date, Dialog, Empty, ErrorNotice, Field, State } from './components';
import Markdown from './Markdown';
import Comments from './Comments';
type Post = Schemas['Post'];
const types: Record<string, string> = { project_log: '项目日志', meeting_note: '会议纪要', sop: '操作规范', article: '内部文章' };

export function KnowledgePage({ me }: { me: Me }) {
  const [params] = useSearchParams(); const projectID = params.get('project_id') ?? '';
  const [search, setSearch] = useState(''), [state, setState] = useState(''), [creating, setCreating] = useState(false);
  const query = useInfiniteQuery({
    queryKey: ['posts', projectID, search, state], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<Post>>('/posts?' + new URLSearchParams({ q: search, state, ...(projectID ? { project_id: projectID } : {}), limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined,
  });
  const items = query.data?.pages.flatMap(page => page.items) ?? [];
  return <><div className="page-heading"><div><span className="eyebrow">KNOWLEDGE THAT MOVES US FORWARD</span><h1>知识与记录<span className="accent">.</span></h1><p>把过程、结论与方法留下，成为下一次工作的起点。</p></div>{me.actions.includes('post.write') && <button className="button primary" onClick={() => setCreating(true)}><Plus size={17} />新建文档</button>}</div>
    <div className="toolbar"><form className="search-field" onSubmit={event => { event.preventDefault(); setSearch(String(new FormData(event.currentTarget).get('q') ?? '').trim()); }}><Search size={18} /><input name="q" aria-label="搜索知识文档" placeholder="搜索标题、标签或正文" maxLength={200} /><button type="submit" aria-label="搜索"><ArrowRight size={17} /></button></form><select aria-label="文档状态" value={state} onChange={event => setState(event.target.value)}><option value="">当前文档</option><option value="draft">草稿</option><option value="published">已发布</option><option value="archived">已归档</option></select></div>
    {projectID && <p className="filter-caption">正在查看关联项目的知识记录。<Link className="text-button" to="/knowledge">查看全部文档</Link></p>}<ErrorNotice error={query.error} />
    {query.isPending ? <Busy label="正在读取知识文档" /> : !items.length && !query.error ? <div className="panel"><Empty title={search || state ? '没有符合条件的文档' : '让经验在团队里流动'} detail={search || state ? '尝试其他关键词或文档状态。' : '从项目日志、会议纪要或操作规范开始，把值得留下的事记录下来。'} /></div> : !!items.length && <div className="knowledge-grid">{items.map(post => <Link to={'/knowledge/' + post.id} className="knowledge-card panel" key={post.id}><div className="knowledge-card-heading"><span><FileText size={17} />{types[post.type ?? 'article']}</span><ArrowUpRight size={18} /></div><h2>{post.title}</h2><p>{post.summary || post.body?.slice(0, 150) || '尚未填写摘要。'}</p><div className="tag-row">{post.tags?.map(tag => <span key={tag} className="tag">{tag}</span>)}</div><div className="knowledge-card-footer"><State state={post.state} /><small>{date(post.updated_at)}</small></div></Link>)}</div>}
    {query.hasNextPage && <div className="list-footer"><span>已显示 {items.length} 份文档</span><button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多文档</button></div>}
    {creating && <PostEditor projectID={projectID} onClose={() => setCreating(false)} />}
  </>;
}

export function KnowledgeDetail({ me }: { me: Me }) {
  const { id = '' } = useParams(); const cache = useQueryClient();
  const [editing, setEditing] = useState(false), [history, setHistory] = useState(false), [action, setAction] = useState<string | null>(null);
  const query = useQuery({ queryKey: ['post', id], queryFn: ({ signal }) => api<Post>('/posts/' + id, { signal }) });
  async function updated() { await cache.invalidateQueries({ queryKey: ['post', id] }); await cache.invalidateQueries({ queryKey: ['posts'] }); }
  if (query.isPending) return <Busy label="正在读取文档" />;
  if (!query.data) return <><Link className="back-link" to="/knowledge"><ArrowLeft size={16} />返回知识文档</Link><ErrorNotice error={query.error} /></>;
  const post = query.data;
  return <><Link className="back-link" to="/knowledge"><ArrowLeft size={16} />返回知识文档</Link><ErrorNotice error={query.error} />
    <div className="page-heading detail-heading"><div><span className="eyebrow">{types[post.type ?? 'article']} / TEAM KNOWLEDGE</span><h1>{post.title}</h1><div className="project-subtitle"><State state={post.state} /><span>{classificationNames[post.classification ?? 'internal']}</span><span>版本 {post.version} · {date(post.updated_at)}</span></div></div><div className="post-actions">{post.can_edit && <button className="button secondary" onClick={() => setEditing(true)}><Pencil size={16} />编辑草稿</button>}{post.can_read_history && <button className="button secondary" onClick={() => setHistory(true)}><History size={16} />修订记录</button>}{post.can_publish && post.state !== 'archived' && post.has_unpublished_changes && <button className="button primary" onClick={() => setAction('publish')}><Check size={17} />内部发布</button>}{post.can_publish && post.state === 'published' && <button className="button secondary" onClick={() => setAction('archive')}><Archive size={16} />归档</button>}{post.can_publish && ['published', 'archived'].includes(post.state) && <button className="button secondary" onClick={() => setAction('draft')}><RotateCcw size={16} />转为草稿</button>}</div></div>
    {post.has_unpublished_changes && <div className="notice neutral" role="status">当前显示未发布草稿；发布后，拥有阅读权限的成员才能看到这次修改。</div>}
    {post.project_id && <div className="post-project-link"><Link className="text-button" to={'/projects/' + post.project_id}>查看关联项目<ArrowUpRight size={15} /></Link></div>}
    <article className="panel post-document">{post.summary && <p className="post-summary">{post.summary}</p>}<Markdown body={post.body ?? ''} /><div className="tag-row">{post.tags?.map(tag => <span className="tag" key={tag}>{tag}</span>)}</div>{!!post.source_refs?.length && <div className="post-sources"><h3>关联文件</h3>{post.source_refs.map(source => <div key={source.id + (source.version_id ?? '')}><FileText size={15} /><span>{source.version_id ? '固定文件版本' : '跟随文件当前版'} · {source.id}</span></div>)}</div>}</article>
    <div className="panel post-discussion"><Comments key={id} type="post" id={id} revisionID={post.current_revision_id} me={me} readonly={!post.can_comment} /></div>
    {editing && <PostEditor post={post} onClose={() => setEditing(false)} />}
    {history && <PostHistory post={post} onClose={() => setHistory(false)} onSuccess={updated} />}
    {action && <PostAction post={post} action={action} onClose={() => setAction(null)} onSuccess={updated} />}
  </>;
}
function PostEditor({ post, projectID = '', onClose }: { post?: Post; projectID?: string; onClose: () => void }) {
  const cache = useQueryClient(), navigate = useNavigate();
  const [body, setBody] = useState(post?.body ?? ''), [preview, setPreview] = useState(false), [selectedProject, setProject] = useState(post?.project_id ?? projectID);
  const projects = useInfiniteQuery({ queryKey: ['knowledge-project-options'], initialPageParam: '', enabled: !post, queryFn: ({ pageParam, signal }) => api<Page<Project>>('/projects?' + new URLSearchParams({ limit: '100', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const options = projects.data?.pages.flatMap(page => page.items).filter(project => !['archived', 'closing', 'cancelled'].includes(project.state)) ?? [];
  const project = options.find(value => value.id === selectedProject);
  const mutation = useMutation({
    mutationFn: (value: Record<string, unknown>) => api<Post>(post ? '/posts/' + post.id : '/posts', { method: post ? 'PATCH' : 'POST', body: value }),
    onSuccess: async result => { cache.setQueryData(['post', result.id], result); await cache.invalidateQueries({ queryKey: ['posts'] }); onClose(); navigate('/knowledge/' + result.id); },
  });
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const form = new FormData(event.currentTarget);
    const value: Record<string, unknown> = { title: form.get('title'), body, summary: form.get('summary'), tags: String(form.get('tags') ?? '').split(/[,，]/).map(tag => tag.trim()).filter(Boolean) };
    if (post) { value.expected_version = post.version; value.base_revision_id = post.current_revision_id; }
    else { value.type = form.get('type'); value.project_id = selectedProject || null; value.classification = selectedProject ? project?.classification ?? 'project_restricted' : 'internal'; }
    mutation.mutate(value);
  }
  return <Dialog title={post ? '编辑文档草稿' : '新建知识文档'} onClose={onClose} busy={mutation.isPending}><form onSubmit={submit}><ErrorNotice error={mutation.error ?? projects.error} /><Field label="文档标题"><input name="title" defaultValue={post?.title} required maxLength={200} autoFocus /></Field>{!post && <div className="form-grid"><Field label="文档类型"><select name="type" defaultValue="project_log">{Object.entries(types).map(([key, name]) => <option value={key} key={key}>{name}</option>)}</select></Field><Field label="关联项目"><select value={selectedProject} onChange={event => setProject(event.target.value)} disabled={projects.isPending}><option value="">公司知识（内部）</option>{selectedProject && !options.some(project => project.id === selectedProject) && <option value={selectedProject}>已选项目</option>}{options.map(project => <option key={project.id} value={project.id}>{project.name}</option>)}</select></Field></div>}
    {!post && projects.hasNextPage && <button type="button" className="text-button" disabled={projects.isFetchingNextPage} onClick={() => void projects.fetchNextPage()}>读取更多可选项目</button>}
    {!post && <p className="form-note">文档将按{selectedProject ? '关联项目的访问范围' : '公司内部的访问范围'}保存；创建后的范围变更须单独核验。</p>}
    <Field label="文档摘要"><textarea name="summary" rows={2} maxLength={20000} defaultValue={post?.summary} placeholder="用几句话说明这份文档的结论或用途。" /></Field>
    <div className="editor-switch"><label htmlFor="post-markdown">Markdown 正文</label><button type="button" className="text-button" onClick={() => setPreview(value => !value)}>{preview ? '继续编辑' : '预览正文'}</button></div>{preview ? <div className="markdown-preview"><Markdown body={body} /></div> : <textarea id="post-markdown" className="markdown-editor" value={body} onChange={event => setBody(event.target.value)} maxLength={20000} rows={12} placeholder="# 结论与背景&#10;&#10;记录要点、依据，以及下一步。" />}
    <small className="muted">支持标题、列表、引用、粗体、斜体、代码和 HTTP/HTTPS 链接；HTML 与外部图片按文本展示。</small>
    <Field label="标签" hint="以逗号分隔，最多20个标签，每个不超过50字符。"><input name="tags" defaultValue={post?.tags?.join('，')} maxLength={1100} placeholder="项目复盘，交付流程" /></Field>
    <div className="dialog-actions"><span className="muted">保存为草稿，保留修订记录</span><button type="button" className="button secondary" onClick={onClose} disabled={mutation.isPending}>取消</button><button className="button primary" disabled={mutation.isPending || (!post && projects.isPending)}>{mutation.isPending ? '正在保存…' : '保存文档草稿'}<ArrowRight size={16} /></button></div>
  </form></Dialog>;
}
function PostAction({ post, action, onClose, onSuccess }: { post: Post; action: string; onClose: () => void; onSuccess: () => Promise<void> }) {
  const title = action === 'publish' ? '内部发布文档' : action === 'archive' ? '归档文档' : '转为草稿';
  const mutation = useMutation({ mutationFn: (reason: string) => api<Post>('/posts/' + post.id + '/publish', { method: 'POST', body: { expected_version: post.version, base_revision_id: post.current_revision_id, action, reason } }), onSuccess: async () => { await onSuccess(); onClose(); } });
  return <Dialog title={title} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(String(new FormData(event.currentTarget).get('reason'))); }}><p className="muted">{action === 'publish' ? '发布固定当前修订，阅读者仍须具备相应访问权限。' : action === 'archive' ? '归档后保留正文与记录，禁止普通编辑和评论。' : '文档将进入草稿状态，仅作者与获授权编辑者可以查阅。'}</p><ErrorNotice error={mutation.error} /><Field label="变更原因"><textarea name="reason" required maxLength={200} rows={3} autoFocus /></Field><div className="dialog-actions"><button type="button" className="button secondary" onClick={onClose} disabled={mutation.isPending}>取消</button><button className="button primary" disabled={mutation.isPending}>{mutation.isPending ? '正在提交…' : title}</button></div></form></Dialog>;
}
function PostHistory({ post, onClose, onSuccess }: { post: Post; onClose: () => void; onSuccess: () => Promise<void> }) {
  const [selected, setSelected] = useState<Schemas['PostRevision'] | null>(null);
  const query = useInfiniteQuery({ queryKey: ['post-history', post.id, post.version], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['PostRevision']>>('/posts/' + post.id + '/revisions?' + new URLSearchParams({ limit: '10', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const mutation = useMutation({ mutationFn: (reason: string) => api<Post>('/posts/' + post.id + '/restore', { method: 'POST', body: { expected_version: post.version, base_revision_id: post.current_revision_id, revision_id: selected?.id, reason } }), onSuccess: async () => { await onSuccess(); onClose(); } });
  return <Dialog title="文档修订记录" onClose={onClose} busy={mutation.isPending}><ErrorNotice error={query.error ?? mutation.error} />{query.isPending ? <Busy /> : <><div className="history-list">{query.data?.pages.flatMap(page => page.items).map(revision => <button key={revision.id} className={'history-choice' + (selected?.id === revision.id ? ' selected' : '')} onClick={() => setSelected(revision)}><span><strong>修订 {revision.seq} · {revision.title || '历史标题未记录'}</strong><small>{date(revision.created_at)}{revision.id === post.current_revision_id ? ' · 当前草稿' : revision.id === post.published_revision_id ? ' · 已发布' : ''}</small></span><ArrowUpRight size={16} /></button>)}</div>{query.hasNextPage && <button className="text-button" onClick={() => void query.fetchNextPage()} disabled={query.isFetchingNextPage}>更早的修订</button>}{selected && <><div className="revision-preview"><Markdown body={selected.body} /></div>{post.can_edit && selected.id !== post.current_revision_id && <form onSubmit={event => { event.preventDefault(); mutation.mutate(String(new FormData(event.currentTarget).get('reason'))); }}><p className="muted">恢复会生成新的草稿修订，并保留现有历史。</p><Field label="恢复原因"><textarea name="reason" required maxLength={200} rows={2} /></Field><button className="button primary" disabled={mutation.isPending}>{mutation.isPending ? '正在恢复…' : '恢复为新草稿'}<RotateCcw size={16} /></button></form>}</>}</>}</Dialog>;
}
