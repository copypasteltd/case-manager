import { useState, type FormEvent } from 'react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ArrowRight, Plus, Pencil, ArrowUpRight, UserPlus } from 'lucide-react';
import { api, type Me, type Page, type Project, type Schemas } from './api';
import { Busy, date, Dialog, Empty, ErrorNotice, Field, State } from './components';

type User = Schemas['User'];
type Milestone = Schemas['Milestone'];
type Concern = Omit<Schemas['Risk'], 'state'> & { state: string; resolution?: string };
type Section = 'milestones' | 'risks' | 'issues' | 'members';
const roles: Record<string, string> = { owner: '负责人', manager: '项目管理', member: '协作成员', viewer: '只读成员' };
const severities: Record<string, string> = { low: '低', medium: '中', high: '高', critical: '严重' };
const states: Record<string, string> = { open: '待处理', mitigating: '应对中', closed: '已关闭', realized: '已转问题', investigating: '调查中', resolved: '已解决' };
const localDateTime = (value?: string | null) => value ? new Date(new Date(value).getTime() + 8 * 3600000).toISOString().slice(0, 16) : '';
const inputTime = (value: FormDataEntryValue | null) => value ? new Date(String(value) + '+08:00').toISOString() : undefined;

function useRefresh(project: Project) {
  const cache = useQueryClient();
  return async () => {
    await Promise.all([
      cache.invalidateQueries({ queryKey: ['project', project.id] }),
      cache.invalidateQueries({ queryKey: ['members', project.id] }),
      cache.invalidateQueries({ queryKey: ['milestones', project.id] }),
      cache.invalidateQueries({ queryKey: ['risks', project.id] }),
      cache.invalidateQueries({ queryKey: ['issues', project.id] }),
      cache.invalidateQueries({ queryKey: ['tasks', project.id] }),
      cache.invalidateQueries({ queryKey: ['dashboard'] }),
    ]);
  };
}
export default function ProjectCollaboration({ section, project, me, users, memberships, readonly }: {
  section: Section; project: Project; me: Me; users: User[];
  memberships: NonNullable<Schemas['ProjectMembers']['memberships']>; readonly: boolean;
}) {
  if (section === 'members') return <Members project={project} me={me} users={users} memberships={memberships} readonly={readonly} />;
  if (section === 'milestones') return <Milestones project={project} me={me} users={users} readonly={readonly} />;
  return <Concerns key={section} kind={section} project={project} me={me} users={users} readonly={readonly} />;
}
function Members({ project, me, users, memberships, readonly }: {
  project: Project; me: Me; users: User[]; memberships: NonNullable<Schemas['ProjectMembers']['memberships']>; readonly: boolean;
}) {
  const [changing, setChanging] = useState<User | 'new' | null>(null);
  const canManage = !readonly && me.actions.includes('project.manage_members');
  return <><div className="section-heading compact"><div><h2>项目成员</h2><p className="muted">项目成员资格与获授业务权限共同决定可执行的操作。</p></div>{canManage && <button className="button primary" onClick={() => setChanging('new')}><UserPlus size={17} />添加成员</button>}</div>
    <div className="panel table-panel"><div className="table-scroll" tabIndex={0} aria-label="项目成员列表"><table><thead><tr><th>成员</th><th>项目角色</th><th>账号状态</th><th>操作</th></tr></thead><tbody>{users.map(user => <tr key={user.id}><td><strong>{user.display_name}</strong>{user.id === me.user.id && <small className="muted"> · 你</small>}</td><td>{roles[memberships.find(value => value.user_id === user.id)?.project_role ?? 'member']}</td><td>{user.status === 'active' ? '有效' : user.status === 'disabled' ? '已停用' : '待激活'}</td><td>{canManage && user.id !== project.owner_id && user.id !== me.user.id ? <button className="text-button" onClick={() => setChanging(user)}><Pencil size={15} />变更</button> : '—'}</td></tr>)}</tbody></table></div></div>
    {changing && <MemberForm project={project} target={changing} users={users} memberships={memberships} onClose={() => setChanging(null)} />}
  </>;
}
function MemberForm({ project, target, users, memberships, onClose }: {
  project: Project; target: User | 'new'; users: User[]; memberships: NonNullable<Schemas['ProjectMembers']['memberships']>; onClose: () => void;
}) {
  const refresh = useRefresh(project);
  const candidates = useQuery({ queryKey: ['member-candidates', project.id], queryFn: ({ signal }) => api<Schemas['ProjectMembers']>('/projects/' + project.id + '/member-candidates', { signal }), enabled: target === 'new' });
  const [action, setAction] = useState('set');
  const mutation = useMutation({
    mutationFn: (form: FormData) => api('/projects/' + project.id + '/members', { method: 'POST', body: {
      expected_version: project.version, user_id: target === 'new' ? form.get('user_id') : target.id,
      action, ...(action === 'set' ? { project_role: form.get('project_role') } : {}), reason: form.get('reason'),
    } }),
    onSuccess: async () => { await refresh(); onClose(); },
  });
  return <Dialog title={target === 'new' ? '添加项目成员' : '变更 ' + target.display_name} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(new FormData(event.currentTarget)); }}>
    <ErrorNotice error={mutation.error ?? candidates.error} />
    {target === 'new' ? <Field label="选择成员"><select name="user_id" required defaultValue="" disabled={candidates.isPending}><option value="" disabled>{candidates.isPending ? '正在读取成员…' : '选择已激活的公司成员'}</option>{candidates.data?.items.filter(user => !users.some(member => member.id === user.id)).map(user => <option key={user.id} value={user.id}>{user.display_name}</option>)}</select></Field> : <Field label="操作"><select value={action} onChange={event => setAction(event.target.value)}><option value="set">变更项目角色</option><option value="remove">移出项目</option></select></Field>}
    {action === 'set' && <Field label="项目角色" hint="此处不会授予新的业务动作；只读成员即使持有其他业务角色，也不能写入本项目。"><select name="project_role" defaultValue={target === 'new' ? 'member' : memberships.find(item => item.user_id === target.id)?.project_role ?? 'member'}><option value="member">协作成员</option><option value="manager">项目管理</option><option value="viewer">只读成员</option></select></Field>}
    {action === 'remove' && <p className="muted">移出前须移交未完成事项；该成员的项目授权和旧会话将失效。</p>}
    <Field label="变更原因"><textarea name="reason" rows={3} maxLength={200} required autoFocus={target !== 'new'} /></Field>
    <div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>取消</button><button className={'button ' + (action === 'remove' ? 'danger' : 'primary')} disabled={mutation.isPending || (target === 'new' && candidates.isPending)}>{mutation.isPending ? '正在提交…' : '保存成员变更'}</button></div>
  </form></Dialog>;
}
function Milestones({ project, me, users, readonly }: { project: Project; me: Me; users: User[]; readonly: boolean }) {
  const [editing, setEditing] = useState<Milestone | 'new' | null>(null);
  const [transition, setTransition] = useState<{ milestone: Milestone; action: string; title: string } | null>(null);
  const refresh = useRefresh(project);
  const query = useInfiniteQuery({
    queryKey: ['milestones', project.id], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<Milestone>>('/projects/' + project.id + '/milestones?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined,
  });
  const items = query.data?.pages.flatMap(page => page.items) ?? [];
  return <><div className="section-heading compact"><div><h2>里程碑</h2><p className="muted">达成以已完成任务与已验收交付为依据。</p></div>{!readonly && me.actions.includes('milestone.create') && <button className="button primary" onClick={() => setEditing('new')}><Plus size={17} />新建里程碑</button>}</div><ErrorNotice error={query.error} />
    {query.isPending ? <Busy label="正在读取里程碑" /> : !items.length && !query.error ? <div className="panel"><Empty title="明确几个关键节点" detail="把目标分成可核验的阶段，让团队看见距离下一步还有多远。" /></div> : !!items.length && <div className="panel table-panel"><div className="table-scroll" tabIndex={0} aria-label="里程碑列表"><table><thead><tr><th>里程碑</th><th>状态</th><th>负责人</th><th>目标时间</th><th>操作</th></tr></thead><tbody>{items.map(m => <tr key={m.id}><td><strong>{m.title}</strong></td><td><State state={m.state} />{m.overdue && <small className="warning-text"> · 已逾期</small>}</td><td>{users.find(user => user.id === m.owner_id)?.display_name ?? '未分配'}</td><td>{date(m.due_at)}</td><td>{!readonly && me.actions.includes('milestone.update') && <div className="inline-actions">{!['achieved', 'cancelled'].includes(m.state) && <><button className="text-button" onClick={() => setEditing(m)}>编辑</button><button className="text-button" onClick={() => setTransition({ milestone: m, action: 'achieve', title: '核验达成' })}>核验达成</button><button className="text-button" onClick={() => setTransition({ milestone: m, action: 'cancel', title: '取消里程碑' })}>取消</button></>}{['achieved', 'cancelled', 'missed'].includes(m.state) && <button className="text-button" onClick={() => setTransition({ milestone: m, action: 'reopen', title: '重开里程碑' })}>重开</button>}</div>}</td></tr>)}</tbody></table></div></div>}
    {query.hasNextPage && <div className="list-footer"><span>已显示 {items.length} 项</span><button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多</button></div>}
    {editing && <MilestoneForm project={project} milestone={editing === 'new' ? undefined : editing} users={users} onClose={() => setEditing(null)} />}
    {transition && <ActionDialog title={transition.title} path={'/milestones/' + transition.milestone.id + '/transition'} version={transition.milestone.version} action={transition.action} onClose={() => setTransition(null)} onSuccess={refresh} />}
  </>;
}
function MilestoneForm({ project, milestone, users, onClose }: { project: Project; milestone?: Milestone; users: User[]; onClose: () => void }) {
  const refresh = useRefresh(project);
  const mutation = useMutation({
    mutationFn: (form: FormData) => api(milestone ? '/milestones/' + milestone.id : '/projects/' + project.id + '/milestones', { method: milestone ? 'PATCH' : 'POST', body: {
      ...(milestone ? { expected_version: milestone.version } : {}), title: form.get('title'), owner_id: form.get('owner_id'), due_at: inputTime(form.get('due_at')),
    } }), onSuccess: async () => { await refresh(); onClose(); },
  });
  return <Dialog title={milestone ? '编辑里程碑' : '新建里程碑'} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(new FormData(event.currentTarget)); }}><ErrorNotice error={mutation.error} /><Field label="里程碑名称"><input name="title" defaultValue={milestone?.title} required maxLength={200} autoFocus /></Field><Field label="负责人"><select name="owner_id" required defaultValue={milestone?.owner_id ?? project.owner_id}>{users.filter(user => user.status === 'active').map(user => <option key={user.id} value={user.id}>{user.display_name}</option>)}</select></Field><Field label="目标时间（香港时间）"><input name="due_at" type="datetime-local" required defaultValue={localDateTime(milestone?.due_at)} /></Field><div className="dialog-actions"><button type="button" className="button secondary" onClick={onClose} disabled={mutation.isPending}>取消</button><button className="button primary" disabled={mutation.isPending}>{mutation.isPending ? '正在保存…' : '保存里程碑'}<ArrowRight size={16} /></button></div></form></Dialog>;
}
function Concerns({ kind, project, me, users, readonly }: { kind: 'risks' | 'issues'; project: Project; me: Me; users: User[]; readonly: boolean }) {
  const entity = kind === 'risks' ? 'risk' : 'issue', label = kind === 'risks' ? '风险' : '问题';
  const [editing, setEditing] = useState<Concern | 'new' | null>(null);
  const query = useInfiniteQuery({
    queryKey: [kind, project.id], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<Concern>>('/projects/' + project.id + '/' + kind + '?' + new URLSearchParams({ limit: '20', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined,
  });
  const items = query.data?.pages.flatMap(page => page.items) ?? [];
  return <><div className="section-heading compact"><div><h2>{label}跟进</h2><p className="muted">{kind === 'risks' ? '提前记录可能发生的影响；风险发生时，转成关联问题继续跟进。' : '明确负责人、解决说明与关闭依据。'}</p></div>{!readonly && me.actions.includes(entity + '.create') && <button className="button primary" onClick={() => setEditing('new')}><Plus size={17} />登记{label}</button>}</div><ErrorNotice error={query.error} />
    {query.isPending ? <Busy label={'正在读取' + label} /> : !items.length && !query.error ? <div className="panel"><Empty title={'暂无' + label + '记录'} detail="需要关注的事项，会在这里持续跟进。" /></div> : !!items.length && <div className="panel table-panel"><div className="table-scroll" tabIndex={0} aria-label={label + '列表'}><table><thead><tr><th>{label}</th><th>状态</th><th>影响</th><th>负责人</th><th>截止时间</th><th>查看</th></tr></thead><tbody>{items.map(item => <tr key={item.id}><td><button className="task-title" onClick={() => setEditing(item)}><strong>{item.title}</strong></button></td><td><State state={item.state} /></td><td>{severities[item.severity ?? 'medium']}</td><td>{users.find(user => user.id === item.owner_id)?.display_name ?? '项目成员'}</td><td>{date(item.due_at)}</td><td><button className="icon-button" aria-label={'查看' + label + ' ' + item.title} onClick={() => setEditing(item)}><ArrowUpRight size={17} /></button></td></tr>)}</tbody></table></div></div>}
    {query.hasNextPage && <div className="list-footer"><span>已显示 {items.length} 项</span><button className="button secondary" onClick={() => void query.fetchNextPage()} disabled={query.isFetchingNextPage}>加载更多</button></div>}
    {editing && <ConcernForm project={project} kind={kind} concern={editing === 'new' ? undefined : editing} users={users} readonly={readonly || !me.actions.includes(entity + (editing === 'new' ? '.create' : '.update'))} onClose={() => setEditing(null)} />}
  </>;
}
function ConcernForm({ project, kind, concern, users, readonly, onClose }: { project: Project; kind: 'risks' | 'issues'; concern?: Concern; users: User[]; readonly: boolean; onClose: () => void }) {
  const refresh = useRefresh(project), risk = kind === 'risks', label = risk ? '风险' : '问题';
  const [state, setState] = useState(concern?.state ?? 'open');
  const frozen = readonly || concern?.state === 'realized';
  const transitions: Record<string, string[]> = risk
    ? { open: ['mitigating', 'closed', 'realized'], mitigating: ['closed', 'realized'], closed: ['open'], realized: [] }
    : { open: ['investigating', 'resolved'], investigating: ['resolved'], resolved: ['closed', 'investigating'], closed: ['open'] };
  const mutation = useMutation({
    mutationFn: (body: Record<string, unknown>) => api(concern ? '/' + kind + '/' + concern.id : '/projects/' + project.id + '/' + kind, { method: concern ? 'PATCH' : 'POST', body }),
    onSuccess: async () => { await refresh(); onClose(); },
  });
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const form = new FormData(event.currentTarget);
    const body: Record<string, unknown> = { title: form.get('title'), description: form.get('description'), owner_id: form.get('owner_id'), severity: form.get('severity') };
    const due = inputTime(form.get('due_at')); if (due) body.due_at = due;
    if (risk) { body.probability = form.get('probability'); body.mitigation = form.get('mitigation'); } else body.resolution = form.get('resolution');
    if (concern) { body.expected_version = concern.version; if (state !== concern.state) { body.state = state; body.reason = form.get('reason'); } }
    mutation.mutate(body);
  }
  return <Dialog title={(concern ? '查看与跟进' : '登记') + label} onClose={onClose} busy={mutation.isPending}><form onSubmit={submit}><ErrorNotice error={mutation.error} />{concern?.issue_id && <p className="notice neutral">此风险已生成关联问题，请到项目的问题列表继续跟进。</p>}<fieldset disabled={frozen || mutation.isPending} className="form-fieldset">
    <Field label={label + '标题'}><input name="title" required maxLength={200} defaultValue={concern?.title} autoFocus /></Field><Field label="情况说明"><textarea name="description" rows={3} required maxLength={20000} defaultValue={concern?.description} /></Field><div className="form-grid"><Field label="负责人"><select name="owner_id" required defaultValue={concern?.owner_id ?? project.owner_id}>{users.filter(user => user.status === 'active').map(user => <option key={user.id} value={user.id}>{user.display_name}</option>)}</select></Field><Field label="影响程度"><select name="severity" defaultValue={concern?.severity ?? 'medium'}>{Object.entries(severities).map(([value, name]) => <option key={value} value={value}>{name}</option>)}</select></Field></div>
    {risk && <Field label="发生概率"><select name="probability" defaultValue={concern?.probability ?? 'medium'}><option value="low">低</option><option value="medium">中</option><option value="high">高</option></select></Field>}
    <Field label="截止时间（香港时间）"><input name="due_at" type="datetime-local" defaultValue={localDateTime(concern?.due_at)} /></Field>
    <Field label={risk ? '应对措施' : '解决说明'}><textarea name={risk ? 'mitigation' : 'resolution'} rows={3} maxLength={20000} defaultValue={risk ? concern?.mitigation : concern?.resolution} required={!risk && ['resolved', 'closed'].includes(state)} /></Field>
    {concern && <Field label="跟进状态"><select value={state} onChange={event => setState(event.target.value)}><option value={concern.state}>{states[concern.state]}</option>{transitions[concern.state]?.map(value => <option key={value} value={value}>{states[value]}</option>)}</select></Field>}
    {concern && state !== concern.state && <Field label="变更原因"><textarea name="reason" rows={2} maxLength={200} required /></Field>}
  </fieldset><div className="dialog-actions"><button type="button" className="button secondary" onClick={onClose} disabled={mutation.isPending}>关闭</button>{!frozen && <button className="button primary" disabled={mutation.isPending}>{mutation.isPending ? '正在保存…' : state === 'realized' && concern?.state !== state ? '创建关联问题并保存' : '保存' + label}</button>}</div></form></Dialog>;
}
function ActionDialog({ title, path, version, action, onClose, onSuccess }: { title: string; path: string; version: number; action: string; onClose: () => void; onSuccess: () => Promise<void> }) {
  const mutation = useMutation({ mutationFn: (reason: string) => api(path, { method: 'POST', body: { expected_version: version, action, reason } }), onSuccess: async () => { await onSuccess(); onClose(); } });
  return <Dialog title={title} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(String(new FormData(event.currentTarget).get('reason'))); }}><ErrorNotice error={mutation.error} /><Field label="变更原因"><textarea name="reason" required maxLength={200} rows={3} autoFocus /></Field><div className="dialog-actions"><button type="button" className="button secondary" disabled={mutation.isPending} onClick={onClose}>取消</button><button className="button primary" disabled={mutation.isPending}>{mutation.isPending ? '正在核验…' : title}</button></div></form></Dialog>;
}
