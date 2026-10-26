import { useRef, useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Me, type Page, type Schemas } from './api';
import { Busy, date, Dialog, Empty, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';
import './project-templates.css';

export type ProjectTemplate = Schemas['ProjectTemplate'];
type TemplateSchema = Schemas['ProjectTemplateSchema'];
type Intent = { path: string; method: 'POST' | 'PATCH'; body: unknown; key: string };
const states = { draft: '草稿', published: '已发布', archived: '已停用' };
const priorities = { low: '低', normal: '普通', high: '高', urgent: '紧急' };
const uncertain = (error: unknown) => error instanceof ApiError && error.status === 0;
const conflict = (error: unknown) => error instanceof ApiError && [404, 409, 412].includes(error.status);

function useTemplates(state: string, q: string, key = '') {
  return useInfiniteQuery({ queryKey: ['project-templates', state, q, key], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<ProjectTemplate>>('/project-templates?' + new URLSearchParams({ state, q, key, limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined });
}
function TemplateState({ value }: { value: ProjectTemplate['state'] }) { return <span className={'badge ' + (value === 'published' ? 'success' : 'neutral')}>{states[value]}</span>; }
export function ProjectTemplateFacts({ value }: { value: ProjectTemplate }) {
  return <section className="template-facts" aria-label="模板固定内容">
    <div className="section-heading compact"><div><h3>{value.name}</h3><p className="muted">{value.key} · 修订 {value.revision} · 记录版本 {value.version}</p></div><TemplateState value={value.state} /></div>
    <p className="prose-text">{value.description || '尚无模板说明。'}</p>
    <p className="form-note">{value.schema.folders.length} 个模板目录 · {value.schema.tasks.length} 项待开始任务 · {value.schema.milestones.length} 个开放里程碑</p>
    <details><summary>核对完整初始化内容</summary><h4>目录</h4><p>标准目录始终包含：合同、交付、工作资料。同名标准目录会复用。</p><ul>{value.schema.folders.map((name, index) => <li key={index}>{name}</li>)}</ul>
      <h4>任务</h4>{value.schema.tasks.length ? <ol>{value.schema.tasks.map((task, index) => <li key={index}><strong>{task.title}</strong> · {priorities[task.priority]} · {task.review_required ? '需要验收' : '直接完成'}<p className="prose-text">{task.body}</p></li>)}</ol> : <p>不生成模板任务。</p>}
      <h4>里程碑</h4><ul>{value.schema.milestones.map((item, index) => <li key={index}>{item.title}</li>)}</ul>
    </details><small>内容摘要</small><code className="full-digest">{value.content_hash}</code>
  </section>;
}

export default function ProjectTemplates({ me }: { me: Me }) {
  const manager = me.actions.includes('config.manage'); const [params] = useSearchParams();
  const [state, setState] = useState(manager ? '' : 'published'), [q, setQ] = useState(''), [search, setSearch] = useState(''), [create, setCreate] = useState(false);
  const family = params.get('key') ?? ''; const query = useTemplates(state, q, family);
  const values = query.error ? [] : query.data?.pages.flatMap(page => page.items) ?? [];
  return <><Link className="back-link" to="/projects">返回项目空间</Link><div className="page-heading"><div><span className="eyebrow">REUSABLE PROJECT STRUCTURE</span><h1>项目模板<span className="accent">.</span></h1><p>发布固定的目录、任务和里程碑结构，创建项目时明确选择修订。</p></div>{manager && <button className="button primary" onClick={() => setCreate(true)}>新建模板</button>}</div>
    <div className="toolbar"><form className="search-field" onSubmit={event => { event.preventDefault(); setQ(search.trim()); }}><input aria-label="搜索项目模板" value={search} onChange={event => setSearch(event.target.value)} maxLength={200} placeholder="名称、系列标识或说明" /><button type="submit" className="text-button">搜索</button></form>{manager && <Field label="模板状态"><select value={state} onChange={event => setState(event.target.value)}><option value="">全部状态</option>{Object.entries(states).map(([value, name]) => <option key={value} value={value}>{name}</option>)}</select></Field>}</div>
    {family && <p className="notice neutral">当前系列：{family} <Link to="/project-templates">查看全部系列</Link></p>}
    <ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !values.length && !query.error ? <Empty title="暂无可用模板" detail={manager ? '建立草稿并核对发布后，项目创建人才能选用。' : '配置管理员发布的模板会显示在这里。'} /> : <div className="panel table-panel"><div className="table-scroll" tabIndex={0} aria-label="项目模板列表"><table><thead><tr><th>模板 / 系列</th><th>固定修订</th><th>状态</th><th>初始化内容</th><th>更新时间</th></tr></thead><tbody>{values.map(value => <tr key={value.id}><td><Link className="row-link" to={'/project-templates/' + value.id}><strong>{value.name}</strong><small>{value.key}</small></Link></td><td>修订 {value.revision}</td><td><TemplateState value={value.state} /></td><td>{value.schema.folders.length} 目录 · {value.schema.tasks.length} 任务 · {value.schema.milestones.length} 里程碑</td><td>{date(value.updated_at)}</td></tr>)}</tbody></table></div></div>}
    {query.hasNextPage && <div className="list-footer"><span>已显示 {values.length} 个固定修订</span><button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多模板</button></div>}
    {create && <TemplateForm onClose={() => setCreate(false)} />}
  </>;
}

export function ProjectTemplateDetail({ me }: { me: Me }) {
  const { id = '' } = useParams(); const manager = me.actions.includes('config.manage');
  const [editing, setEditing] = useState(false), [action, setAction] = useState<'publish' | 'retire' | 'revise' | null>(null);
  const query = useQuery({ queryKey: ['project-template', id], queryFn: ({ signal }) => api<ProjectTemplate>('/project-templates/' + id, { signal }) });
  const value = query.error ? undefined : query.data;
  return <><Link className="back-link" to="/project-templates">返回项目模板</Link><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : value && <>
    <div className="page-heading"><div><span className="eyebrow">FIXED TEMPLATE REVISION</span><h1>{value.name}</h1><p>系列 {value.key} · 固定修订 {value.revision}</p></div>{manager && <div className="template-actions">{value.state === 'draft' ? <><button className="button secondary" onClick={() => setEditing(true)}>编辑草稿</button><button className="button primary" onClick={() => setAction('publish')}>发布修订</button></> : <button className="button secondary" onClick={() => setAction('revise')}>建立新修订</button>}{value.state !== 'archived' && <button className="button danger-outline" onClick={() => setAction('retire')}>停用修订</button>}</div>}</div>
    <div className="panel content-panel"><ProjectTemplateFacts value={value} /><p className="form-note">{value.published_at ? '发布于 ' + date(value.published_at) : value.state === 'published' ? '旧资料未记录发布人或时间，不能据此推断。' : '此修订尚未发布。'}{value.retired_at && ' · 停用于 ' + date(value.retired_at)}</p><Link className="text-button" to={'/project-templates?key=' + encodeURIComponent(value.key)}>查看此系列所有可见修订</Link>{value.based_on_id && manager && <Link className="text-button" to={'/project-templates/' + value.based_on_id}>查看来源修订</Link>}</div>
    {value.state === 'published' && <p className="notice neutral">此修订已固定。请在新建项目时选择；发布新修订不会更改已创建项目。</p>}
    {manager && <TemplateHistory id={id} />}{editing && <TemplateForm initial={value} onClose={() => setEditing(false)} />}{action && <TemplateAction value={value} action={action} onClose={() => setAction(null)} />}
  </>}</>;
}
function TemplateHistory({ id }: { id: string }) {
  const query = useInfiniteQuery({ queryKey: ['project-template-history', id], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Schemas['ProjectTemplateEvent']>>('/project-templates/' + id + '/history?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const items = query.error ? [] : query.data?.pages.flatMap(page => page.items) ?? [];
  const names = { created: '建立草稿', revised: '从旧修订建立草稿', updated: '编辑草稿', published: '发布修订', retired: '停用修订' };
  return <section className="section-block"><h2>修订操作记录</h2><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !items.length && !query.error ? <p className="muted">尚无可验证的管理记录。旧资料不会补造历史。</p> : items.map(item => <article key={item.id} className="panel content-panel"><h3>{names[item.kind]} · 记录版本 {item.version}</h3><p>{item.reason}</p><p className="muted">{date(item.created_at)} · 操作者标识 {item.actor_id}</p><details><summary>查看此次固定内容</summary><ProjectTemplateFacts value={item.snapshot} /></details></article>)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多操作记录</button>}</section>;
}

function useTemplateCommand(onSuccess: (value: ProjectTemplate) => void) {
  const cache = useQueryClient(); const intent = useRef<Intent>();
  const mutation = useMutation({ mutationFn: (value: Intent) => api<ProjectTemplate>(value.path, { method: value.method, body: value.body, key: value.key }), onSuccess: async value => { await Promise.all(['project-template', 'project-templates', 'project-template-history'].map(key => cache.invalidateQueries({ queryKey: [key] }))); onSuccess(value); } });
  return { mutation, send: (path: string, method: Intent['method'], body: unknown) => { intent.current = { path, method, body, key: crypto.randomUUID() }; mutation.mutate(intent.current); }, reconcile: () => { if (intent.current) mutation.mutate(intent.current); } };
}
function TemplateForm({ initial, onClose }: { initial?: ProjectTemplate; onClose: () => void }) {
  const navigate = useNavigate(); const [basis, setBasis] = useState(initial), [fresh, setFresh] = useState(false), [comparison, setComparison] = useState<ProjectTemplate>();
  const [name, setName] = useState(initial?.name ?? ''), [key, setKey] = useState(initial?.key ?? ''), [description, setDescription] = useState(initial?.description ?? ''), [reason, setReason] = useState('');
  const [folders, setFolders] = useState(initial?.schema.folders.join('\n') ?? ''), [milestones, setMilestones] = useState(initial?.schema.milestones.map(item => item.title).join('\n') ?? '');
  const [tasks, setTasks] = useState<TemplateSchema['tasks']>(() => structuredClone(initial?.schema.tasks ?? [])), [compareError, setCompareError] = useState<unknown>();
  const command = useTemplateCommand(value => { onClose(); navigate('/project-templates/' + value.id); }); const { mutation } = command;
  const unknown = uncertain(mutation.error), changed = conflict(mutation.error) && !comparison;
  const setTask = (index: number, patch: Partial<TemplateSchema['tasks'][number]>) => setTasks(values => values.map((task, i) => i === index ? { ...task, ...patch } : task));
  async function compare() { if (!initial) return; try { setComparison(await api<ProjectTemplate>('/project-templates/' + initial.id)); setCompareError(undefined); } catch (error) { setCompareError(error); } }
  function submit(event: FormEvent) { event.preventDefault(); const schema: TemplateSchema = { folders: folders ? folders.split('\n') : [], tasks, milestones: milestones ? milestones.split('\n').map(title => ({ title })) : [] };
    setComparison(undefined); command.send('/project-templates' + (basis ? '/' + basis.id : ''), basis ? 'PATCH' : 'POST', { name, description, schema, reason, ...(basis ? { expected_version: basis.version } : { key }) });
  }
  return <Dialog title={initial ? '编辑模板草稿' : '新建项目模板'} onClose={onClose} busy={mutation.isPending}><form onSubmit={submit}><ErrorNotice error={mutation.error ?? compareError} />
    <fieldset disabled={mutation.isPending || unknown} className="template-fields"><Field label="模板名称"><input required maxLength={200} value={name} onChange={event => setName(event.target.value)} autoFocus /></Field><Field label="系列标识" hint="小写字母开头，仅用字母、数字、连字符；创建后固定。"><input required pattern="[a-z][a-z0-9-]{0,63}" maxLength={64} readOnly={!!initial} value={key} onChange={event => setKey(event.target.value)} /></Field><Field label="模板说明"><textarea rows={3} maxLength={2000} value={description} onChange={event => setDescription(event.target.value)} /></Field><Field label="模板目录" hint="每行一个目录，最多100项。不能含路径或重复名称；标准目录同名会复用。"><textarea rows={4} value={folders} onChange={event => setFolders(event.target.value)} maxLength={20100} /></Field>
    <div className="section-heading compact"><h3>初始化任务（{tasks.length}/200）</h3><button type="button" className="button secondary" disabled={tasks.length >= 200} onClick={() => setTasks(values => [...values, { title: '', body: '', priority: 'normal', review_required: false }])}>添加模板任务</button></div>
    {tasks.map((task, index) => <section className="template-task" key={index} aria-label={'模板任务 ' + (index + 1)}><Field label={'任务 ' + (index + 1) + ' 标题'}><input required maxLength={200} value={task.title} onChange={event => setTask(index, { title: event.target.value })} /></Field><Field label={'任务 ' + (index + 1) + ' 说明'}><textarea rows={2} maxLength={20000} value={task.body} onChange={event => setTask(index, { body: event.target.value })} /></Field><Field label={'任务 ' + (index + 1) + ' 优先级'}><select value={task.priority} onChange={event => setTask(index, { priority: event.target.value as typeof task.priority })}>{Object.entries(priorities).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></Field><label className="checkbox-row"><input type="checkbox" checked={task.review_required} onChange={event => setTask(index, { review_required: event.target.checked })} />需要验收</label><button type="button" className="button secondary" onClick={() => setTasks(values => values.filter((_, i) => i !== index))}>移除任务 {index + 1}</button></section>)}
    <Field label="初始化里程碑" hint="每行一个标题，最多100项；负责人为项目负责人，日期需在项目中安排。"><textarea rows={4} maxLength={20100} value={milestones} onChange={event => setMilestones(event.target.value)} /></Field><Field label="本次操作原因"><textarea required rows={2} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} /></Field></fieldset>
    <p className="form-note">模板属于公司通用配置，请使用可共享的中性内容。这里只生成逻辑目录、未分配任务与开放里程碑；文件、成员权限和业务审批仍需分别处理。</p>
    {conflict(mutation.error) && initial && <><button type="button" className="button secondary" disabled={mutation.isPending} onClick={() => void compare()}>读取当前修订并比较</button>{comparison && <><ProjectTemplateFacts value={comparison} />{comparison.state === 'draft' ? <label className="checkbox-row"><input type="checkbox" checked={basis?.version === comparison.version} onChange={event => setBasis(event.target.checked ? comparison : initial)} />我已比较当前内容，保留上方输入并基于记录版本 {comparison.version} 保存</label> : <p className="notice neutral">当前修订已发布或停用，不能再保存草稿。</p>}</>}</>}
    {unknown && <p className="notice neutral">结果未确认。输入已保留；请原样核对本次操作，避免建立另一份草稿。</p>}<MFAConfirmation onFresh={setFresh} />
    <div className="dialog-actions"><button type="button" className="button secondary" onClick={onClose} disabled={mutation.isPending}>关闭</button>{unknown ? <button type="button" className="button primary" disabled={mutation.isPending || !fresh} onClick={command.reconcile}>原样核对结果</button> : <button className="button primary" disabled={mutation.isPending || !fresh || changed || (conflict(mutation.error) && comparison?.state !== 'draft') || (conflict(mutation.error) && basis?.version !== comparison?.version)}>保存草稿</button>}</div>
  </form></Dialog>;
}
function TemplateAction({ value, action, onClose }: { value: ProjectTemplate; action: 'publish' | 'retire' | 'revise'; onClose: () => void }) {
  const [basis] = useState(value), [fresh, setFresh] = useState(false), [confirmed, setConfirmed] = useState(false), [reason, setReason] = useState(''); const navigate = useNavigate();
  const names = { publish: '发布修订', retire: '停用修订', revise: '建立新修订' };
  const descriptions = { publish: '发布后内容固定，项目创建人可以选用此修订。后续修改须建立新的草稿修订。', retire: '停用后不能新选用此修订，待提交的秘书计划会拒绝执行。已经创建的项目及其任务、目录、里程碑继续保留。', revise: '从此修订复制中性结构并建立独立草稿。旧修订仍可使用，直到另行明确停用；既有项目不会自动升级。' };
  const command = useTemplateCommand(result => { onClose(); if (action === 'revise') navigate('/project-templates/' + result.id); }); const { mutation } = command; const unknown = uncertain(mutation.error);
  return <Dialog title={names[action]} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); command.send('/project-templates/' + basis.id + '/' + action, 'POST', { expected_version: basis.version, content_hash: basis.content_hash, reason }); }}><ErrorNotice error={mutation.error} /><ProjectTemplateFacts value={basis} /><p className="notice neutral">{descriptions[action]}</p><Field label="本次操作原因"><textarea required rows={3} maxLength={200} value={reason} onChange={event => setReason(event.target.value)} disabled={mutation.isPending || unknown} /></Field><label className="checkbox-row"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} disabled={mutation.isPending || unknown} />我已核对固定修订 {basis.revision} 的完整内容和上述影响</label><MFAConfirmation onFresh={setFresh} />{unknown && <p className="notice neutral">结果未确认，原样核对不会建立第二个操作。</p>}{conflict(mutation.error) && <p className="notice neutral">依据已变化，请关闭窗口，重新读取当前修订后明确操作。</p>}<div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>关闭</button>{unknown ? <button type="button" className="button primary" disabled={!fresh || mutation.isPending} onClick={command.reconcile}>原样核对结果</button> : <button className={'button ' + (action === 'retire' ? 'danger' : 'primary')} disabled={!fresh || !confirmed || mutation.isPending || conflict(mutation.error)}>{names[action]}</button>}</div></form></Dialog>;
}

export function ProjectTemplateSelection({ value, onChange }: { value: ProjectTemplate | null; onChange: (value: ProjectTemplate | null) => void }) {
  const [open, setOpen] = useState(false), [search, setSearch] = useState(''), [q, setQ] = useState('');
  return <section className="template-selection" aria-label="项目初始化模板"><h3>初始化模板</h3>{value ? <><ProjectTemplateFacts value={value} /><button type="button" className="button secondary" onClick={() => onChange(null)}>改用标准目录</button></> : <p className="muted">使用标准目录，不生成模板任务或里程碑。</p>}<button type="button" className="button secondary" onClick={() => setOpen(!open)}>{open ? '收起模板列表' : '选择项目模板'}</button>{open && <><Field label="查找已发布模板"><input value={search} maxLength={200} onChange={event => setSearch(event.target.value)} /></Field><button className="button secondary" type="button" onClick={() => setQ(search.trim())}>查找模板</button><PublishedTemplateOptions q={q} onSelect={selected => { onChange(selected); setOpen(false); }} /></>}</section>;
}
function PublishedTemplateOptions({ q, onSelect }: { q: string; onSelect: (value: ProjectTemplate) => void }) {
  const query = useTemplates('published', q); const values = query.error ? [] : query.data?.pages.flatMap(page => page.items) ?? [];
  return <><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !values.length && !query.error ? <p className="muted">暂无已发布模板，可继续使用标准目录。</p> : values.map(value => <article className="template-option" key={value.id}><strong>{value.name} · 修订 {value.revision}</strong><p className="muted">{value.key} · {value.schema.tasks.length} 任务 · {value.schema.milestones.length} 里程碑</p><button className="button secondary" type="button" onClick={() => onSelect(value)}>选择 {value.key} 修订 {value.revision}</button></article>)}{query.hasNextPage && <button className="button secondary" type="button" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多模板</button>}</>;
}
