import { useRef, useState } from 'react';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { ArrowDown, ArrowUp, GitPullRequest, Plus, X } from 'lucide-react';
import { api, ApiError, ensureDownloadSession, type Me, type Page, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';

type Definition = Schemas['WorkflowDefinition'];
type StepInput = Schemas['WorkflowStepInput'];
type Kind = Definition['kind'];
const names: Record<Kind, string> = { contract: '合同', purchase: '采购', purchase_variance: '采购差异重审', claim: '报销', change: '项目变更' };
const amount = (value: string) => value.includes('.') ? value.replace(/0+$/, '').replace(/\.$/, '') : value;
const moment = (value: string) => new Intl.DateTimeFormat('zh-HK', { timeZone: 'Asia/Hong_Kong', dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
function useCommandKey() {
  const state = useRef({ body: '', key: '' });
  return (body: unknown) => { const text = JSON.stringify(body); if (state.current.body !== text) state.current = { body: text, key: crypto.randomUUID() }; return state.current.key; };
}
function useDefinitions(kind: string, history?: string) {
  return useInfiniteQuery({ queryKey: ['workflow-definitions', kind, history ?? 'current'], initialPageParam: '',
    queryFn: ({ pageParam, signal }) => api<Page<Definition>>('/workflow-definitions' + (history ? '/' + history + '/history' : '') + '?' + new URLSearchParams({ limit: '20', ...(kind ? { kind } : {}), ...(pageParam ? { cursor: pageParam } : {}) }), { signal }),
    getNextPageParam: page => page.next_cursor ?? undefined });
}
export function WorkflowDefinitionsPage({ me }: { me: Me }) {
  if (!me.actions.includes('config.manage')) return <Empty title="没有制度管理权限" detail="请联系制度负责人核对授权。" />;
  return <DefinitionList me={me} />;
}
function DefinitionList({ me }: { me: Me }) {
  const [kind, setKind] = useState(''), [editing, setEditing] = useState<Definition | 'new' | null>(null), [history, setHistory] = useState<Definition | null>(null);
  const query = useDefinitions(kind);
  const entries = query.error ? [] : [...new Map(query.data?.pages.flatMap(page => page.items).map(item => [item.key, item])).values()];
  return <><div className="page-heading"><div><div className="eyebrow">GOVERNANCE / APPROVALS</div><h1>审批制度</h1><p>明确审批人、金额区间与顺序，保留每次发布依据。</p></div><button className="button primary" onClick={() => setEditing('new')}><Plus size={18} />制定审批规则</button></div>
    <div className="notice neutral">启用制度供后续申请提交时匹配。修订会保留旧制度与原有审批依据；未匹配或存在歧义的申请不能自动通过。</div>
    <div className="list-toolbar"><Field label="业务类型"><select value={kind} onChange={event => setKind(event.target.value)}><option value="">全部业务</option>{Object.entries(names).map(([value, name]) => <option value={value} key={value}>{name}</option>)}</select></Field></div>
    <ErrorNotice error={query.error} />{query.isPending ? <Busy label="正在读取审批制度" /> : !query.error && !entries.length ? <Empty title="尚无此类审批制度" detail="先确定审批人、金额口径与本人审批政策，再发布适用规则。" /> : entries.map(rule => <RuleCard rule={rule} key={rule.id} actions={<><button className="button secondary" onClick={() => setEditing(rule)}>发布新修订</button><button className="text-button" onClick={() => setHistory(rule)}>查看修订记录</button></>} />)}
    {query.hasNextPage && <div className="list-footer"><span>已显示 {entries.length} 项规则</span><button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更多规则</button></div>}
    {editing && <DefinitionEditor rule={editing === 'new' ? undefined : editing} initialKind={kind} me={me} onClose={() => setEditing(null)} />}
    {history && <DefinitionHistory rule={history} onClose={() => setHistory(null)} />}
  </>;
}
function RuleCard({ rule, actions }: { rule: Definition; actions?: React.ReactNode }) {
  return <article className="panel workflow-card"><div className="section-heading"><div><span className="eyebrow">{names[rule.kind]} · 修订 {rule.version}</span><h2>{rule.name}</h2></div><span className={'badge ' + (rule.enabled ? 'success' : 'neutral')}>{rule.enabled ? '已启用' : '已停用'}</span></div>
    <p className="muted workflow-code">{rule.key} · {moment(rule.created_at)}</p><dl className="file-facts"><div><dt>适用类别</dt><dd>{rule.category === '*' ? '全部类别' : rule.category}</dd></div><div><dt>金额区间</dt><dd>{rule.currency} {amount(rule.min_amount)}（含）{rule.max_amount === null ? ' 起，不设上限' : ' 至 ' + amount(rule.max_amount) + '（不含）'}</dd></div><div><dt>本人审批</dt><dd>{rule.self_approval_policy === 'deny' ? '禁止审批本人申请' : '制度明确允许'}</dd></div><div><dt>申请人撤回</dt><dd>{rule.withdrawal_allowed ? '审批中允许撤回' : '审批中不允许撤回'}</dd></div></dl>
    <ol className="workflow-sequence" aria-label="固定审批顺序">{rule.steps.map(step => <li key={step.seq}><span className="step-number">{step.seq}</span><div><strong>{step.title}</strong><p>{step.assigned_user_name} · {step.requires_mfa ? '须近期两步验证' : '须本人有效会话'}</p>{!step.assigned_user_active && <span className="badge warning">审批人已停用，需修订制度</span>}</div></li>)}</ol>
    <p className="workflow-reason"><strong>发布依据</strong>{rule.reason}</p>{actions && <div className="post-actions">{actions}</div>}
  </article>;
}
function DefinitionHistory({ rule, onClose }: { rule: Definition; onClose: () => void }) {
  const query = useDefinitions('', rule.id);
  return <Dialog title={rule.name + ' · 修订记录'} onClose={onClose}><p className="muted">规则 {rule.key} 的完整发布记录。所示审批人姓名和账号状态为当前资料。</p><ErrorNotice error={query.error} />{query.isPending ? <Busy /> : !query.error && query.data?.pages.flatMap(page => page.items).map(item => <RuleCard rule={item} key={item.id} />)}{query.hasNextPage && <button className="button secondary" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>加载更早修订</button>}</Dialog>;
}
function DefinitionEditor({ rule, initialKind, me, onClose }: { rule?: Definition; initialKind: string; me: Me; onClose: () => void }) {
  const [base] = useState(rule), [name, setName] = useState(rule?.name ?? ''), [code, setCode] = useState(rule?.key ?? '');
  const [kind, setKind] = useState(rule?.kind ?? initialKind), [category, setCategory] = useState(rule?.category ?? ''), [currency, setCurrency] = useState(rule?.currency ?? '');
  const [minimum, setMinimum] = useState(rule ? amount(rule.min_amount) : ''), [maximum, setMaximum] = useState(rule?.max_amount ? amount(rule.max_amount) : '');
  const [enabled, setEnabled] = useState(rule?.enabled ?? false), [self, setSelf] = useState(rule?.self_approval_policy ?? ''), [withdraw, setWithdraw] = useState(rule ? rule.withdrawal_allowed ? 'allow' : 'deny' : '');
  const [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const [steps, setSteps] = useState<(StepInput & { localID: string })[]>(rule?.steps.map(step => ({ title: step.title, assigned_user_id: step.assigned_user_id, requires_mfa: step.requires_mfa, localID: crypto.randomUUID() })) ?? [{ title: '', assigned_user_id: '', requires_mfa: true, localID: crypto.randomUUID() }]);
  const users = useInfiniteQuery({ queryKey: ['users', 'workflow-selector'], initialPageParam: '', enabled: me.actions.includes('user.read'),
    queryFn: ({ pageParam, signal }) => api<Page<Schemas['User']>>('/users?' + new URLSearchParams({ limit: '50', ...(pageParam ? { cursor: pageParam } : {}) }), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const people = users.error ? [] : [...new Map(users.data?.pages.flatMap(page => page.items).map(item => [item.id, item])).values()];
  const cache = useQueryClient(), key = useCommandKey();
  const mutation = useMutation({ mutationFn: async () => {
    await ensureDownloadSession();
    const body: Schemas['WorkflowDefinitionInput'] = { expected_version: base?.version ?? 0, key: code, name, kind: kind as Kind, category, currency, min_amount: minimum, max_amount: maximum || null, enabled,
      self_approval_policy: self as 'deny' | 'allow', withdrawal_allowed: withdraw === 'allow', steps: steps.map(({ title, assigned_user_id, requires_mfa }) => ({ title, assigned_user_id, requires_mfa })), reason };
    return api<Definition>('/workflow-definitions', { method: 'POST', body, key: key(body) });
  }, onSuccess: async () => { await Promise.all([cache.invalidateQueries({ queryKey: ['workflow-definitions'] }), cache.invalidateQueries({ queryKey: ['me'] })]); onClose(); } });
  const conflict = mutation.error instanceof ApiError && ['VERSION_CONFLICT', 'AUTHORIZATION_CHANGED'].includes(mutation.error.code);
  const updateStep = (localID: string, change: Partial<StepInput>) => setSteps(value => value.map(step => step.localID === localID ? { ...step, ...change } : step));
  function move(index: number, offset: number) { setSteps(value => { const result = [...value]; [result[index], result[index + offset]] = [result[index + offset], result[index]]; return result; }); }
  return <Dialog title={base ? '发布审批规则新修订' : '制定审批规则'} onClose={onClose} busy={mutation.isPending}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}>
    <ErrorNotice error={mutation.error} />{conflict && <p className="notice neutral">制度或授权已变化。已保留填写内容，请核对最新修订后重新填写本次发布。</p>}
    <div className="form-grid"><Field label="规则名称"><input required maxLength={200} value={name} onChange={event => setName(event.target.value)} /></Field><Field label="稳定规则代码" hint="3–64个小写字母、数字、点、下划线或短横线，以字母开头。"><input required readOnly={!!base} pattern="[a-z][a-z0-9_.-]{2,63}" maxLength={64} value={code} onChange={event => setCode(event.target.value)} /></Field>
      <Field label="业务类型"><select required value={kind} onChange={event => setKind(event.target.value)}><option value="" disabled>选择业务</option>{Object.entries(names).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></Field><Field label="路由类别" hint="填写准确类别代码；仅填写 * 时表示全部类别。"><input required maxLength={50} value={category} onChange={event => setCategory(event.target.value)} /></Field>
      <Field label="币种" hint="使用币种代码，如 HKD；按对应币种精度校验。"><input required pattern="[A-Z]{3}" maxLength={3} autoCapitalize="characters" value={currency} onChange={event => setCurrency(event.target.value.toUpperCase())} /></Field><Field label="金额下界（含）"><input required inputMode="decimal" pattern="(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?" value={minimum} onChange={event => setMinimum(event.target.value)} /></Field><Field label="金额上界（不含）" hint="留空明确表示不设上限；上界须大于下界。"><input inputMode="decimal" pattern="(0|[1-9][0-9]{0,13})(\.[0-9]{1,6})?" value={maximum} onChange={event => setMaximum(event.target.value)} /></Field></div>
    <section className="workflow-editor-steps" aria-label="设置审批顺序"><div className="section-heading"><h3>顺序审批人</h3><span className="muted">{steps.length} / 10 步</span></div><p className="muted">按顺序指定准确成员，同一流程不可重复指定同一个人。</p><ErrorNotice error={users.error} />{steps.map((step, index) => <div className="workflow-step-editor" key={step.localID}><div className="section-heading"><strong>第 {index + 1} 步</strong><div className="row-actions"><button type="button" className="icon-button" aria-label={'上移第 ' + (index + 1) + ' 步'} disabled={index === 0} onClick={() => move(index, -1)}><ArrowUp size={16} /></button><button type="button" className="icon-button" aria-label={'下移第 ' + (index + 1) + ' 步'} disabled={index === steps.length - 1} onClick={() => move(index, 1)}><ArrowDown size={16} /></button><button type="button" className="icon-button" aria-label={'移除第 ' + (index + 1) + ' 步'} disabled={steps.length === 1} onClick={() => setSteps(value => value.filter(item => item.localID !== step.localID))}><X size={16} /></button></div></div>
      <Field label={'第 ' + (index + 1) + ' 步名称'}><input required maxLength={200} value={step.title} onChange={event => updateStep(step.localID, { title: event.target.value })} /></Field>
      {me.actions.includes('user.read') ? <Field label={'第 ' + (index + 1) + ' 步审批人'}><select required value={step.assigned_user_id} onChange={event => updateStep(step.localID, { assigned_user_id: event.target.value })}><option value="" disabled>选择审批成员</option>{step.assigned_user_id && !people.some(person => person.id === step.assigned_user_id) && <option value={step.assigned_user_id}>{base?.steps.find(item => item.assigned_user_id === step.assigned_user_id)?.assigned_user_name ?? '当前指定成员'}（尚未载入）</option>}{people.filter(person => person.status === 'active' || person.id === step.assigned_user_id).map(person => <option key={person.id} value={person.id}>{person.display_name}{person.status !== 'active' ? '（账号不可用）' : ''}</option>)}</select></Field> : <Field label={'第 ' + (index + 1) + ' 步审批人标识'} hint="填写已核对的本组织成员 UUID。"><input required value={step.assigned_user_id} onChange={event => updateStep(step.localID, { assigned_user_id: event.target.value })} /></Field>}
      <label className="checkbox-row"><input type="checkbox" checked={step.requires_mfa} onChange={event => updateStep(step.localID, { requires_mfa: event.target.checked })} />此步决策要求最近十分钟内两步验证</label></div>)}
      <div className="post-actions"><button type="button" className="button secondary" disabled={steps.length >= 10} onClick={() => setSteps(value => [...value, { localID: crypto.randomUUID(), title: '', assigned_user_id: '', requires_mfa: true }])}><Plus size={16} />添加下一步</button>{users.hasNextPage && <button type="button" className="text-button" disabled={users.isFetchingNextPage} onClick={() => void users.fetchNextPage()}>加载更多成员</button>}</div></section>
    <div className="form-grid"><Field label="本人审批政策"><select required value={self} onChange={event => setSelf(event.target.value)}><option value="" disabled>明确选择公司政策</option><option value="deny">禁止审批本人申请</option><option value="allow">明确允许审批本人申请</option></select></Field><Field label="审批中撤回政策"><select required value={withdraw} onChange={event => setWithdraw(event.target.value)}><option value="" disabled>明确选择公司政策</option><option value="allow">允许申请人撤回</option><option value="deny">不允许申请人撤回</option></select></Field></div>
    {self === 'allow' && <p className="notice warning">这条规则允许指定审批人处理自己发起的申请。请核对公司已确定的授权政策。</p>}
    <label className="checkbox-row"><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)} />启用此修订，供后续送审匹配</label><Field label="发布依据"><textarea required maxLength={200} rows={3} value={reason} onChange={event => setReason(event.target.value)} /></Field>
    <p className="form-note">相同类型、币种、类别与金额区间不能有重叠的启用规则。发布时将核对审批人和当前修订。</p><MFAConfirmation onFresh={setFresh} />
    <div className="dialog-actions"><button type="button" className="button secondary" onClick={onClose} disabled={mutation.isPending}>取消</button><button className="button primary" disabled={mutation.isPending || !fresh || conflict}><GitPullRequest size={16} />{enabled ? '发布为启用制度' : '发布为停用修订'}</button></div>
  </form></Dialog>;
}
