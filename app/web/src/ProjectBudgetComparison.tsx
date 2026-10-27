import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { comparisonPath, currentBudgetComparison, currentComparisonEntries } from '@copypaste/contracts/budget-comparison';
import { api, type Schemas } from './api';
import { Busy, date, Empty, ErrorNotice, Field } from './components';
import { projectAmount } from './ProjectOverview';

export default function ProjectBudgetComparison({ projectID, initialCurrency = '' }: { projectID: string; initialCurrency?: string }) {
  const [currency, setCurrency] = useState(initialCurrency), [applied, setApplied] = useState(initialCurrency);
  const [line, setLine] = useState(''), [state, setState] = useState<'incurred' | 'draft'>('incurred');
  const summary = useQuery({ queryKey: ['project-budget-comparison', projectID, applied], enabled: !!applied,
    queryFn: async ({ signal }) => currentBudgetComparison(await api<Schemas['BudgetComparison']>(comparisonPath(projectID, applied), { signal }), projectID, applied) });
  const report = summary.error || summary.isFetching ? undefined : summary.data;
  const sources = useInfiniteQuery({ queryKey: ['project-budget-comparison-sources', projectID, applied, report?.basis_hash, report?.as_of, line, state],
    enabled: !!report, initialPageParam: '', queryFn: async ({ pageParam, signal }) => currentComparisonEntries(
      await api<Schemas['BudgetComparisonEntries']>(comparisonPath(projectID, applied, { basis: report!.basis_hash, line, state, cursor: pageParam, limit: 5 }), { signal }), report!, line, state),
    getNextPageParam: page => page.next_cursor ?? undefined });
  const entries = sources.error ? [] : sources.data?.pages.flatMap(page => page.items) ?? [];
  return <section className="panel content-panel budget-comparison" aria-label="预算与实际支出对比">
    <div className="section-heading compact"><div><div className="eyebrow">PROJECT / BUDGET RECONCILIATION</div><h2>预算与实际支出对比</h2><p className="muted">按当前已实施预算的完整类别与期间，核对当前可读费用的项目份额。</p></div></div>
    <form className="toolbar" onSubmit={event => { event.preventDefault(); setLine(''); if (currency === applied) void summary.refetch(); else setApplied(currency); }}>
      <Field label="对比币种"><input required pattern="[A-Z]{3}" maxLength={3} placeholder="填写三位币种代码" value={currency} onChange={event => setCurrency(event.target.value.toUpperCase())} /></Field>
      <button className="button primary" disabled={summary.isFetching}>查询预算对比</button>
    </form>
    <ErrorNotice error={summary.error} />{summary.isFetching ? <Busy /> : !applied ? <p className="notice neutral">填写一个已配置币种，读取当前预算与费用。不同币种分别查询。</p> : report && <>
      <p className="notice neutral">此处仅包含当前有权读取的费用，不代表项目全部费用或可用资金。按发生日匹配预算期间（包含起止日）；预测、付款和未确认费用不计入实际支出。</p>
      <p className="form-note">读取于 {date(report.as_of)} · 项目版本 {report.project_version} · {report.budget ? `当前预算修订 ${report.budget.revision}` : '尚无当前批准预算'}</p>
      <dl className="comparison-totals"><div><dt>当前预算总额</dt><dd>{report.budget ? report.currency + ' ' + projectAmount(report.budget.total) : '尚无批准预算'}</dd></div><div><dt>预算内可见净支出</dt><dd>{report.currency} {projectAmount(report.matched.net_amount)}</dd></div><div><dt>预算内差额</dt><dd>{report.budget_variance === null ? '无预算依据' : report.currency + ' ' + projectAmount(report.budget_variance)}</dd></div><div><dt>预算外可见净支出</dt><dd>{report.currency} {projectAmount(report.outside.net_amount)}</dd></div></dl>
      <div className="table-scroll" role="region" aria-label="预算明细对比表" tabIndex={0}><table><thead><tr><th>类别／预算期间</th><th>批准预算</th><th>原费用份额</th><th>冲销份额</th><th>净发生支出</th><th>预算差额</th><th>核对状态</th></tr></thead><tbody>
        {report.lines.map(item => <tr key={item.index}><td>{item.category}<br /><span className="muted">{item.period_start} — {item.period_end}</span></td><td>{projectAmount(item.budget_amount)}</td><td>{projectAmount(item.expense_amount)}</td><td>{projectAmount(item.reversal_amount)}</td><td>{projectAmount(item.net_amount)}</td><td>{projectAmount(item.variance)}</td><td>{item.over_budget ? '可见费用已超预算' : '按可见费用核对'}<br />{item.entry_count} 笔已发生 · {item.unconfirmed_count} 笔未确认<button className="text-button" onClick={() => { setLine(String(item.index)); setState('incurred'); }}>核对第{item.index + 1}项费用</button></td></tr>)}
      </tbody></table></div>{!report.lines.length && <p className="notice neutral">{report.budget ? '当前批准预算已明确清空；费用仍保留在预算外。' : '尚无批准预算；当前可读费用均列入预算外，不推断预算为零。'}</p>}
      <p>全期间可见发生支出 {report.currency} <strong>{projectAmount(report.lifetime.net_amount)}</strong> · 已发生 {report.lifetime.entry_count} 笔 · 未确认 {report.lifetime.unconfirmed_count} 笔。</p>
      <div className="section-heading compact"><h3>费用来源核对</h3></div><div className="toolbar"><Field label="核对范围"><select value={line} onChange={event => setLine(event.target.value)}><option value="">全部可见费用</option><option value="outside">预算外费用</option>{report.lines.map(item => <option key={item.index} value={String(item.index)}>第{item.index + 1}项 · {item.category} · {item.period_start}</option>)}</select></Field><Field label="费用确认状态"><select value={state} onChange={event => setState(event.target.value as typeof state)}><option value="incurred">已发生费用</option><option value="draft">未确认费用</option></select></Field></div>
      <ErrorNotice error={sources.error} />{sources.error && <button className="button secondary" onClick={() => void summary.refetch()}>重新读取预算对比</button>}
      {sources.isPending ? <Busy /> : !sources.error && !entries.length ? <Empty title="没有符合条件的可读费用" detail="此结果仅表示当前授权范围，不代表项目总支出为零。" /> : <div className="table-scroll" role="region" aria-label="预算费用来源表" tabIndex={0}><table><thead><tr><th>规范费用／发生日</th><th>状态／归属</th><th>项目份额</th><th>发生支出贡献</th></tr></thead><tbody>{entries.map(entry => <tr key={entry.id}><td><Link to={'/costs/' + entry.id}>{entry.title}</Link><br /><span className="muted">{entry.occurred_on ?? '发生日未填'} · {entry.category || '尚未分类'}</span></td><td>{entry.state === 'draft' ? '未确认' : entry.entry_kind === 'reversal' ? '冲销记录' : entry.state === 'reversed' ? '原费用（已有冲销）' : '已确认'}<br />{entry.line_index === null ? '预算外' : '预算第' + (entry.line_index + 1) + '项'}</td><td>{entry.currency} {projectAmount(entry.share_amount)}</td><td>{entry.state === 'draft' ? '未计入' : entry.currency + ' ' + projectAmount(entry.signed_amount)}</td></tr>)}</tbody></table></div>}
      {sources.hasNextPage && <button className="button secondary" disabled={sources.isFetchingNextPage} onClick={() => void sources.fetchNextPage()}>加载更多对比费用</button>}
      <p className="form-note">每页重新核验权限并读取当前费用。预算或项目依据变化需重新查询；此对比不阻止采购，也不生成付款。</p>
    </>}
  </section>;
}
