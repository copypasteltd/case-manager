import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { budgetStatus } from '@copypaste/contracts/budget';
import { api, type Me, type Page, type Schemas } from './api';
import { Busy, Empty, ErrorNotice, Field, date } from './components';

type Entry = Schemas['BudgetEntry'];
export default function AgentBudget({ me }: { me: Me }) {
  const [scope, setScope] = useState('mine'), [id, setID] = useState(''), [month, setMonth] = useState('');
  const [selection, setSelection] = useState('scope=mine');
  const manager = me.actions.includes('policy.manage');
  const summary = useQuery({ queryKey: ['agent-budget', selection], queryFn: ({ signal }) => api<Schemas['BudgetSummary']>('/agent-budget?' + selection, { signal }) });
  const entries = useInfiniteQuery({ queryKey: ['agent-budget', selection, 'entries'], initialPageParam: '', queryFn: ({ pageParam, signal }) => api<Page<Entry>>('/agent-budget/entries?' + selection + '&limit=20' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''), { signal }), getNextPageParam: page => page.next_cursor ?? undefined });
  const s = summary.data, rows = entries.data?.pages.flatMap(page => page.items) ?? [];
  return <><div className="page-heading"><div><div className="eyebrow">SECRETARY / HISTORY</div><h1>秘书用量历史</h1><p>查看已保留的费用预留记录。此页只读，秘书接单与执行无需配置费用额度。</p></div><Link className="primary" to="/conversations">前往公司秘书</Link></div>
    <section className="panel section-block"><h2>历史账本查询</h2><form className="budget-filter" onSubmit={event => { event.preventDefault(); setSelection(new URLSearchParams({ scope, ...(id && !['mine', 'organization'].includes(scope) ? { scope_id: id.trim() } : {}), ...(month ? { month } : {}) }).toString()); }}>
      <Field label="账本范围"><select value={scope} onChange={event => setScope(event.target.value)}><option value="mine">本人</option>{manager && <><option value="organization">整个组织</option><option value="user">指定员工</option><option value="project">指定项目</option><option value="skill">指定Skill</option></>}</select></Field>
      {!['mine', 'organization'].includes(scope) && <Field label="范围标识"><input value={id} onChange={event => setID(event.target.value)} required maxLength={64} /></Field>}
      <Field label="UTC月份"><input type="month" value={month} onChange={event => setMonth(event.target.value)} /></Field><button className="secondary" type="submit">查询历史</button></form>
      <ErrorNotice error={summary.error ?? entries.error} />{summary.isLoading ? <Busy /> : s && <><h2>{s.month} · {scope === 'mine' ? '本人' : '选定范围'}</h2><div className="budget-metrics">
        {[['历史运行预留', s.reserved_tokens], ['历史已记用量', s.charged_tokens], ['历史待核对', s.pending_tokens]].map(([label, value]) => <div key={label}><span>{label}</span><strong>{typeof value === 'number' ? value.toLocaleString() : value}</strong></div>)}</div>
        {s.unbudgeted_runs > 0 && <p role="status">本月另有 {s.unbudgeted_runs} 次运行未建立费用预留，不在此历史账本中。</p>}
        <p className="muted">历史记录保持原样，未知用量保留待核对标识。这份账本不代表供应商账单。</p></>}
    </section>
    <section className="panel section-block"><h2>历史运行记录</h2>{!entries.isLoading && !rows.length && <Empty title="本范围暂无历史预留记录" detail="费用控制已停用。可直接前往公司秘书发起会话。" />}<div className="table-scroll"><table><thead><tr><th>运行 / Skill</th><th>归属</th><th>历史状态</th><th>接单估算</th><th>运行预留</th><th>已记 / 待核对</th></tr></thead><tbody>{rows.map(e => <tr key={e.id}><td><code>{e.id}</code><br />{e.skill_id}<br />{date(e.created_at)}</td><td>员工 <code>{e.actor_id}</code><br />项目 <code>{e.project_id ?? '个人会话'}</code></td><td>{budgetStatus[e.status]}<br />{e.run_state}</td><td>{e.estimated_tokens.toLocaleString()}</td><td>{e.reserved_tokens.toLocaleString()}</td><td>{e.charged_tokens.toLocaleString()} / {e.pending_tokens.toLocaleString()}{e.evidence_reference && <><br />{e.evidence_reference} · {e.reconciliation_reason}</>}</td></tr>)}</tbody></table></div>{entries.hasNextPage && <button className="secondary" disabled={entries.isFetchingNextPage} onClick={() => void entries.fetchNextPage()}>加载更多历史记录</button>}</section>
  </>;
}
