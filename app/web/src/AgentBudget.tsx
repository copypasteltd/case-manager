import { useRef, useState } from 'react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { budgetLabels, budgetPolicyInput, budgetReconcileInput, budgetStatus } from '@copypaste/contracts/budget';
import { api, type Me, type Page, type Schemas } from './api';
import { Busy, Dialog, Empty, ErrorNotice, Field, date } from './components';
import { MFAConfirmation } from './MFAConfirmation';

type Entry = Schemas['BudgetEntry'];
export default function AgentBudget({ me }: { me: Me }) {
  const [scope, setScope] = useState('mine'), [id, setID] = useState(''), [month, setMonth] = useState('');
  const [selection, setSelection] = useState('scope=mine'), [edit, setEdit] = useState(false), [reconcile, setReconcile] = useState<Entry>();
  const cache = useQueryClient(), manager = me.actions.includes('policy.manage');
  const summary = useQuery({ queryKey: ['agent-budget', selection], queryFn: ({ signal }) => api<Schemas['BudgetSummary']>('/agent-budget?' + selection, { signal }) });
  const entries = useInfiniteQuery({ queryKey: ['agent-budget', selection, 'entries'], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Entry>>('/agent-budget/entries?' + selection + '&limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  async function changed() { setEdit(false); setReconcile(undefined); await cache.invalidateQueries({ queryKey: ['agent-budget'] }); }
  const s = summary.data, rows = entries.data?.pages.flatMap(page => page.items) ?? [];
  return <><div className="page-heading"><div><div className="eyebrow">SECRETARY / TOKEN ACCOUNT</div><h1>秘书额度与用量</h1><p>按UTC自然月预留与结算。输入Token包含缓存，输出包含推理；均只计算一次。</p></div>{manager && <button className="primary" onClick={() => setEdit(true)}>配置月度额度</button>}</div>
    <section className="panel section-block"><form className="budget-filter" onSubmit={event => { event.preventDefault(); setSelection(new URLSearchParams({ scope, ...(id && !['mine', 'organization'].includes(scope) ? { scope_id: id.trim() } : {}), ...(month ? { month } : {}) }).toString()); }}>
      <Field label="账本范围"><select value={scope} onChange={event => setScope(event.target.value)}><option value="mine">本人</option>{manager && <><option value="organization">整个组织</option><option value="user">指定员工</option><option value="project">指定项目</option><option value="skill">指定Skill</option></>}</select></Field>
      {!['mine', 'organization'].includes(scope) && <Field label="范围标识"><input value={id} onChange={event => setID(event.target.value)} required maxLength={64} /></Field>}
      <Field label="UTC月份"><input type="month" value={month} onChange={event => setMonth(event.target.value)} /></Field><button className="secondary" type="submit">查询账本</button></form>
      <ErrorNotice error={summary.error ?? entries.error} />{summary.isLoading ? <Busy /> : s && <><h2>{s.month} · {scope === 'mine' ? '本人' : '选定范围'}</h2><div className="budget-metrics">
        {[['当前策略额度', s.limit_tokens ?? '未配置'], ['运行预留', s.reserved_tokens], ['已记用量（含待核对）', s.charged_tokens], ['其中待核对', s.pending_tokens], ['可用额度', s.available_tokens ?? '未配置']].map(([label, value]) => <div key={label}><span>{label}</span><strong>{typeof value === 'number' ? value.toLocaleString() : value}</strong></div>)}</div>
        {s.unbudgeted_runs > 0 && <p role="status">本月另有 {s.unbudgeted_runs} 次升级前运行未纳入账本，未补造历史用量。</p>}
        <p className="muted">已启动但未收到用量的运行按预留量计入待核对。取消不会假定供应商免费；这份Token账本不代表供应商账单或金额硬上限。</p></>}
    </section>
    <section className="panel section-block"><h2>运行账本</h2>{!entries.isLoading && !rows.length && <Empty title="本范围暂无预留记录" detail="新运行会先检查四类额度，预留成功后才进入队列。" />}<div className="table-scroll"><table><thead><tr><th>运行 / Skill</th><th>归属</th><th>状态</th><th>接单估算</th><th>运行预留</th><th>已记 / 待核对</th><th>操作</th></tr></thead><tbody>{rows.map(e => <tr key={e.id}><td><code>{e.id}</code><br />{e.skill_id}<br />{date(e.created_at)}</td><td>员工 <code>{e.actor_id}</code><br />项目 <code>{e.project_id ?? '个人会话'}</code></td><td>{budgetStatus[e.status]}<br />{e.run_state}</td><td>{e.estimated_tokens.toLocaleString()}</td><td>{e.reserved_tokens.toLocaleString()}</td><td>{e.charged_tokens.toLocaleString()} / {e.pending_tokens.toLocaleString()}{e.evidence_reference && <><br />凭据：{e.evidence_reference}<br />{e.reconciliation_reason}</>}</td><td>{manager && e.status === 'pending' && <button className="secondary" onClick={() => setReconcile(e)}>核对用量</button>}</td></tr>)}</tbody></table></div>{entries.hasNextPage && <button className="secondary" disabled={entries.isFetchingNextPage} onClick={() => void entries.fetchNextPage()}>加载更多记录</button>}</section>
    {edit && <BudgetDialog policy={s?.policy ?? null} onClose={() => setEdit(false)} onSaved={changed} />}{reconcile && <BudgetDialog entry={reconcile} onClose={() => setReconcile(undefined)} onSaved={changed} />}
  </>;
}
function BudgetDialog({ policy, entry, onClose, onSaved }: { policy?: Schemas['BudgetPolicy'] | null; entry?: Entry; onClose(): void; onSaved(): Promise<void> }) {
  const [values, setValues] = useState<Record<keyof typeof budgetLabels, string>>({ organization_tokens: policy ? String(policy.organization_tokens) : '', user_tokens: policy ? String(policy.user_tokens) : '', project_tokens: policy ? String(policy.project_tokens) : '', skill_tokens: policy ? String(policy.skill_tokens) : '' });
  const [input, setInput] = useState(''), [output, setOutput] = useState(''), [evidence, setEvidence] = useState(''), [reason, setReason] = useState(''), [fresh, setFresh] = useState(false);
  const key = useRef(crypto.randomUUID()), mutation = useMutation({ mutationFn: () => entry ? api('/agent-budget/entries/' + entry.id + '/reconcile', { method: 'POST', key: key.current, body: budgetReconcileInput(input, output, evidence, reason, entry.run_version) }) : api('/agent-budget/policies', { method: 'POST', key: key.current, body: budgetPolicyInput(values, reason, policy?.version ?? 0) }), onSuccess: onSaved });
  return <Dialog title={entry ? '凭证核对运行用量' : '配置秘书月度Token额度'} onClose={onClose}><form onSubmit={event => { event.preventDefault(); mutation.mutate(); }}><ErrorNotice error={mutation.error} />
    {entry ? <><p>仅限已终止且用量未知的运行。填写已核对的实际输入、输出Token；有凭据确认零用量时可明确填0。</p><Field label="实际输入Token"><input inputMode="numeric" value={input} onChange={e => setInput(e.target.value)} required /></Field><Field label="实际输出Token"><input inputMode="numeric" value={output} onChange={e => setOutput(e.target.value)} required /></Field><Field label="核对凭据编号"><input value={evidence} maxLength={120} onChange={e => setEvidence(e.target.value)} required /></Field></> : <><p>同一组织的全部入口共用额度。每位员工、每个项目、每项Skill分别累计；0表示停止相应范围接新运行。已接单预留保留。</p>{(Object.keys(budgetLabels) as (keyof typeof budgetLabels)[]).map(k => <Field key={k} label={budgetLabels[k]}><input inputMode="numeric" value={values[k]} onChange={e => setValues(old => ({ ...old, [k]: e.target.value }))} required /></Field>)}</>}
    <Field label={entry ? '核对原因' : '发布原因'}><input value={reason} maxLength={120} onChange={e => setReason(e.target.value)} required /></Field><MFAConfirmation onFresh={setFresh} /><div className="dialog-actions"><button type="button" className="secondary" onClick={onClose}>返回</button><button className="primary" disabled={!fresh || mutation.isPending}>确认{entry ? '核对' : '发布'}</button></div></form></Dialog>;
}
