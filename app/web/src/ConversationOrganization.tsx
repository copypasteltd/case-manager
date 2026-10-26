import { useRef, useState } from 'react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me, type Page, type Schemas } from './api';
import { Busy, Empty, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';

type Plan = Schemas['AgentOrganizationPlan'];
type Entry = Schemas['AgentOrganizationEntry'];
type Action = Schemas['AgentOrganizationAction'];
const states = { proposed: '待本人批准', approved: '已批准，等待整理', committed: '资料已整理', revoked: '已撤销' };
const kinds = { 'folder.create': '新建目录', 'folder.move': '调整目录', 'file.move': '调整文件' };
function location(path: Entry[], entry: Entry) { return [...path].reverse().map(e => e.name).concat(entry.name).join(' / '); }

export function ConversationOrganization({ conversation: c, me, onReference, referenceDisabled }: {
  conversation: Schemas['Conversation']; me: Me; onReference?(plan: Plan): void; referenceDisabled?: boolean;
}) {
  const [creating, setCreating] = useState(false);
  const enabled = c.type === 'private' && c.creator_id === me.user.id && me.actions.includes('agent.plan.read');
  const query = useQuery({ queryKey: ['organization-plans', c.id, c.current_revision_id, me.authorization_version], enabled,
    queryFn: ({ signal }) => api<Schemas['AgentOrganizationPlanList']>('/conversations/' + c.id + '/organization-plans', { signal }), refetchInterval: 5000, gcTime: 0 });
  if (!enabled) return null;
  return <section className="panel conversation-summary"><h2>资料整理计划</h2>
    <p className="form-note">先核对逐项位置和名称，再由本人通过两步验证批准。最多 10 项，整批成功后才提交；有效期 30 分钟。</p>
    <ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && <>
      {query.data?.items.map(p => <OrganizationCard key={p.id + ':' + p.version} plan={p} onRefresh={() => query.refetch()} onReference={onReference} referenceDisabled={referenceDisabled} />)}
      {!query.data?.items.length && <Empty title="尚无资料整理计划" detail="可以选择目录拟定计划，或请秘书在此会话中拟定。" />}
      {query.data?.has_more && <p>当前显示最近 50 份整理计划。</p>}
    </>}
    {c.state === 'active' && me.actions.includes('agent.plan.propose') && me.actions.includes('agent.run') && <button className="button secondary" onClick={() => setCreating(v => !v)}>{creating ? '收起资料整理' : '拟定资料整理计划'}</button>}
    {creating && <OrganizationBuilder conversation={c} onDone={() => { setCreating(false); void query.refetch(); }} />}
    <button className="text-button" onClick={() => void query.refetch()}>重新核对整理计划</button>
  </section>;
}

function OrganizationBuilder({ conversation: c, onDone }: { conversation: Schemas['Conversation']; onDone(): void }) {
  const [folderID, setFolderID] = useState(''), [source, setSource] = useState<Entry>(), [target, setTarget] = useState<Entry>();
  const [kind, setKind] = useState<Action['kind']>('folder.create'), [name, setName] = useState(''), [reason, setReason] = useState('');
  const [actions, setActions] = useState<{ action: Action; label: string }[]>([]), [blocked, setBlocked] = useState(false), [localError, setLocalError] = useState<unknown>();
  const intent = useRef<{ key: string; body: Schemas['AgentOrganizationPlanCreate'] }>();
  const roots = useInfiniteQuery({ queryKey: ['organization-roots', c.id], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<Schemas['Folder']>>('/folders?limit=20' + (c.project_id ? '&project_id=' + c.project_id : '') + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined, gcTime: 0 });
  const directory = useQuery({ queryKey: ['organization-directory', c.id, folderID], enabled: !!folderID,
    queryFn: ({ signal }) => api<Schemas['AgentDirectory']>('/conversations/' + c.id + '/directories/' + folderID, { signal }), gcTime: 0 });
  const create = useMutation({ mutationFn: async () => {
    intent.current ??= { key: crypto.randomUUID(), body: { expected_version: c.version, context_security_version: c.context_security_version, actions: actions.map(v => v.action), reason } };
    const result = await api<Plan>('/conversations/' + c.id + '/organization-plans', { method: 'POST', body: intent.current.body, key: intent.current.key });
    if (result.conversation_id !== c.id || result.state !== 'proposed' || result.preview.length !== intent.current.body.actions.length) throw new Error('计划回执待核对，请使用原请求重新核对。');
  }, onSuccess: onDone, onError: () => setBlocked(true), retry: false });
  const disabled = !!intent.current || create.isPending;
  function add() {
    setLocalError(undefined);
    const parent = directory.error ? undefined : directory.data?.folder;
    if (actions.length >= 10 || (kind === 'folder.create' ? !parent || !name.trim() : !source || (!target && (!name.trim() || name === source.name)))) { setLocalError(new Error('请选择资料，并填写新名称或指定目标目录。')); return; }
    const entry = kind === 'folder.create' ? parent! : source!;
    const action: Action = { kind, id: entry.id, expected_version: entry.version,
      ...(name.trim() && (kind === 'folder.create' || name !== entry.name) ? { name } : {}),
      ...(kind !== 'folder.create' && target ? { target_folder_id: target.id, expected_target_version: target.version } : {}) };
    setActions(old => [...old, { action, label: kinds[kind] + '：' + entry.name + (action.name ? ' → ' + action.name : '') + (target && kind !== 'folder.create' ? '；移至 ' + target.name : '') }]);
    setName(''); setSource(undefined); setTarget(undefined);
  }
  return <div className="conversation-source-preview"><h3>选择资料与整理动作</h3><ErrorNotice error={create.error || localError || roots.error || directory.error} />
    <Field label="资料根目录"><select disabled={disabled} value={folderID} onChange={e => setFolderID(e.target.value)}><option value="">请选择资料目录</option>
      {roots.data?.pages.flatMap(p => p.items).filter(v => (v.project_id ?? null) === (c.project_id ?? null)).map(v => <option key={v.id} value={v.id}>{v.name}</option>)}
      {folderID && !roots.data?.pages.some(p => p.items.some(v => v.id === folderID)) && <option value={folderID}>{directory.data?.folder.name ?? '当前子目录'}</option>}
    </select></Field>
    {roots.hasNextPage && <button className="text-button" disabled={disabled || roots.isFetchingNextPage} onClick={() => void roots.fetchNextPage()}>加载更多根目录</button>}
    {folderID && directory.isPending && <Busy />}
    {!directory.error && directory.data && <><p>当前目录：{directory.data.folder.name}</p><div className="actions">
      {directory.data.folder.parent_id && <button className="button secondary" disabled={disabled} onClick={() => setFolderID(directory.data!.folder.parent_id!)}>返回上一级目录</button>}
      <button className="button secondary" disabled={disabled} onClick={() => { setSource(directory.data!.folder); setKind('folder.move'); setName(directory.data!.folder.name); setTarget(undefined); }}>调整当前目录</button>
      {source && <button className="button secondary" disabled={disabled} onClick={() => setTarget(directory.data!.folder)}>将当前目录设为移动目标</button>}
    </div>
      {directory.data.items.map(item => <div className="list-row" key={item.id}><span>{item.kind === 'folder' ? '目录' : '文件'} · {item.name}</span><div className="actions">
        {item.kind === 'folder' && <button className="text-button" disabled={disabled} onClick={() => setFolderID(item.id)}>进入目录 {item.name}</button>}
        <button className="text-button" disabled={disabled} onClick={() => { setSource(item); setKind(item.kind === 'file' ? 'file.move' : 'folder.move'); setName(item.name); setTarget(undefined); }}>选择 {item.name}</button>
      </div></div>)}
      {directory.data.has_more && <p className="notice neutral">当前只列出前 50 个可见直接子项，尚未盘点整个目录。</p>}
    </>}
    <Field label="整理动作"><select value={kind} disabled={disabled} onChange={e => { setKind(e.target.value as Action['kind']); setName(''); setSource(undefined); setTarget(undefined); }}><option value="folder.create">在当前目录下新建目录</option><option value="folder.move">重命名或移动目录</option><option value="file.move">重命名或移动文件</option></select></Field>
    {kind !== 'folder.create' && <p>已选资料：{source?.name ?? '请从上方选择'}；目标目录：{target?.name ?? '保持原位置'}</p>}
    <Field label={kind === 'folder.create' ? '新目录名称' : '新的资料名称'}><input maxLength={200} disabled={disabled} value={name} onChange={e => setName(e.target.value)} /></Field>
    <button className="button secondary" disabled={disabled || actions.length >= 10 || (kind !== 'folder.create' && source?.kind !== (kind === 'file.move' ? 'file' : 'folder'))} onClick={add}>加入本次整理计划</button>
    <ol>{actions.map((v, index) => <li key={index}>{v.label} <button className="text-button" disabled={disabled} onClick={() => setActions(old => old.filter((_, i) => i !== index))}>移除此项</button></li>)}</ol>
    <Field label="整理原因"><input required maxLength={200} disabled={disabled} value={reason} onChange={e => setReason(e.target.value)} /></Field>
    {blocked && <p className="notice neutral">请求内容已保留。请重新核对计划；重试会使用相同内容和请求标识。</p>}
    <button className="button primary" disabled={create.isPending || !actions.length || !reason.trim()} onClick={() => create.mutate()}>{blocked ? '以原请求核对整理计划' : '保存待批准整理计划'}</button>
  </div>;
}

function OrganizationCard({ plan: p, onRefresh, onReference, referenceDisabled }: { plan: Plan; onRefresh(): Promise<unknown>; onReference?(plan: Plan): void; referenceDisabled?: boolean }) {
  const cache = useQueryClient(), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false), [blocked, setBlocked] = useState(false);
  const intent = useRef<{ action: 'approve' | 'commit' | 'revoke'; body: Schemas['AgentPlanAction']; key: string }>();
  const command = useMutation({ mutationFn: async (action: 'approve' | 'commit' | 'revoke') => {
    if (intent.current && intent.current.action !== action) throw new Error('请先核对前次操作结果。');
    intent.current ??= { action, body: { expected_version: p.version, plan_hash: p.plan_hash, reason }, key: crypto.randomUUID() };
    const result = await api<Plan>('/organization-plans/' + p.id + '/' + action, { method: 'POST', body: intent.current.body, key: intent.current.key });
    if (result.id !== p.id || result.plan_hash !== p.plan_hash || (action === 'commit' && (result.state !== 'committed' || !result.operation_id || result.results.length !== p.preview.length))) throw new Error('整理结果尚未确认，请重新核对计划。');
  }, onSuccess: async () => { await onRefresh(); await Promise.all([cache.invalidateQueries({ queryKey: ['folders'] }), cache.invalidateQueries({ queryKey: ['files'] }), cache.invalidateQueries({ queryKey: ['organization-directory'] })]); }, onError: () => setBlocked(true), retry: false });
  const disabled = command.isPending || !reason.trim();
  return <article className="conversation-source-preview"><h3>{p.preview.length} 项资料整理 · {states[p.state]}</h3><p>{p.reason}</p>
    <ol>{p.preview.map((v, i) => <li key={i}><strong>{kinds[v.action.kind]}</strong><p>原位置：{location(v.path, v.source)}（版本 {v.source.version}）</p>
      <p>变更后：{v.action.kind === 'folder.create' ? location(v.path, v.source) + ' / ' + v.action.name : v.target ? location(v.target_path, v.target) + ' / ' + (v.action.name ?? v.source.name) : [...v.path].reverse().map(e => e.name).concat(v.action.name ?? v.source.name).join(' / ')}</p>
      <p>资料分类：{v.source.classification === 'internal' ? '公司内部' : v.source.classification === 'project_restricted' ? '项目受限' : '严格受限'}{v.target ? '；目标目录版本 ' + v.target.version : ''}</p>
    </li>)}</ol>
    <p>有效期至 {new Date(p.expires_at).toLocaleString('zh-HK', { timeZone: 'Asia/Hong_Kong' })}（香港）</p>
    <details><summary>核对准确计划标识</summary><p>{p.id} · 版本 {p.version}</p><code className="full-digest">{p.plan_hash}</code></details>
    {onReference && <button className="button secondary" disabled={referenceDisabled} onClick={() => onReference(p)}>引用这份整理计划</button>}
    {p.blocked_reason && <p className="notice neutral">会话、权限、资料或有效期已变化。请重新拟定并批准；仍可撤销当前计划。</p>}
    {p.state === 'committed' && <div role="status"><p>本批资料已整理。批次回执：<code>{p.operation_id}</code></p>{p.results.map((r, i) => <p key={r.operation_id}>第 {i + 1} 项：{r.data.name} · 业务回执 <code>{r.operation_id}</code></p>)}</div>}
    {(p.can_approve || p.can_commit || p.can_revoke) && <><ErrorNotice error={command.error} /><Field label="整理操作说明"><input maxLength={200} disabled={!!intent.current || command.isPending} value={reason} onChange={e => setReason(e.target.value)} /></Field>
      {(p.can_approve || p.can_commit) && <MFAConfirmation onFresh={setFresh} />}
      {blocked ? <><p>操作结果待核对。请重新读取计划；重试将使用原请求。</p><button className="button secondary" disabled={command.isPending || (intent.current?.action !== 'revoke' && !fresh)} onClick={() => command.mutate(intent.current!.action)}>以原请求核对整理回执</button></> : <div className="actions">
        {p.can_approve && <button className="button primary" disabled={disabled || !fresh} onClick={() => command.mutate('approve')}>批准这份整理计划</button>}
        {p.can_commit && <button className="button primary" disabled={disabled || !fresh} onClick={() => command.mutate('commit')}>按已批准计划整理资料</button>}
        {p.can_revoke && <button className="button secondary" disabled={disabled} onClick={() => command.mutate('revoke')}>撤销整理计划</button>}
      </div>}
    </>}
  </article>;
}
