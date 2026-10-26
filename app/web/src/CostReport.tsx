import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { ArrowUpRight } from 'lucide-react';
import { api, type Me, type Schemas } from './api';
import { Busy, date, Empty, ErrorNotice, Field } from './components';
import { money } from './ContractDisplay';
import { useCostProjects } from './CostDisplay';

function currentMonth() {
  const parts = new Intl.DateTimeFormat('en', { timeZone: 'Asia/Hong_Kong', year: 'numeric', month: '2-digit' }).formatToParts();
  return parts.find(part => part.type === 'year')!.value + '-' + parts.find(part => part.type === 'month')!.value;
}
function period(month: string) {
  const [year, part] = month.split('-').map(Number);
  return { from: month + '-01', to: String(part === 12 ? year + 1 : year).padStart(4, '0') + '-' + String(part === 12 ? 1 : part + 1).padStart(2, '0') + '-01' };
}
type Selection = { month: string; scope: string; currency: string; category: string };
function parameters(selection: Selection) {
  return new URLSearchParams({ ...period(selection.month), ...(selection.scope === 'company' ? { company_only: 'true' } : selection.scope ? { project_id: selection.scope } : {}), ...(selection.currency ? { currency: selection.currency } : {}), ...(selection.category ? { category: selection.category } : {}) });
}
export default function CostReport({ me }: { me: Me }) {
  const [selection, setSelection] = useState<Selection>({ month: currentMonth(), scope: '', currency: '', category: '' }), [applied, setApplied] = useState(selection), projects = useCostProjects();
  const query = useQuery({ queryKey: ['cost-report', applied], enabled: me.actions.includes('report.read'), queryFn: ({ signal }) => api<Schemas['CostReport']>('/reports/expenses?kind=incurred&' + parameters(applied), { signal }) });
  if (!me.actions.includes('report.read')) return <Empty title="没有财务报表权限" detail="请联系财务负责人核对业务范围与授权。" />;
  const report = query.error ? undefined : query.data, names = projects.data?.pages.flatMap(page => page.items) ?? [];
  return <><div className="page-heading"><div><div className="eyebrow">FINANCE / MONTHLY EXPENSES</div><h1>月度发生支出</h1><p>核对真实发生的费用、冲销及项目分摊。</p></div><Link className="button secondary" to="/costs">查阅费用台账<ArrowUpRight size={17} /></Link></div>
    <form className="panel cost-report-filters" onSubmit={event => { event.preventDefault(); setApplied({ ...selection, category: selection.category.trim(), currency: selection.currency.trim() }); }}><Field label="统计月份"><input required type="month" min="2000-01" max="9998-12" value={selection.month} onChange={event => setSelection({ ...selection, month: event.target.value })} /></Field><Field label="分摊范围"><select value={selection.scope} onChange={event => setSelection({ ...selection, scope: event.target.value })}><option value="">全部可见费用</option><option value="company">仅公司分摊</option>{names.map(project => <option value={project.id} key={project.id}>{project.name}</option>)}</select></Field><Field label="币种"><input pattern="[A-Z]{3}|^$" maxLength={3} placeholder="全部币种" value={selection.currency} onChange={event => setSelection({ ...selection, currency: event.target.value.toUpperCase() })} /></Field><Field label="费用类别"><input maxLength={50} placeholder="全部类别" value={selection.category} onChange={event => setSelection({ ...selection, category: event.target.value })} /></Field><button className="button primary" disabled={query.isFetching}>查询支出</button>{projects.hasNextPage && <button className="text-button" type="button" disabled={projects.isFetchingNextPage} onClick={() => void projects.fetchNextPage()}>加载更多项目</button>}</form>
    <ErrorNotice error={query.error ?? projects.error} />{query.isPending ? <Busy /> : report && <><p className="notice neutral">发生日自 {report.from} 起，至 {report.to} 前；以香港时间划分期间。仅汇总当前授权可见的已确认费用，原费用与冲销分别保留。按各原币列示，尚未设置统一折算。</p><div className="section-heading"><h2>{applied.month} · {applied.scope === 'company' ? '公司分摊' : applied.scope ? names.find(p => p.id === applied.scope)?.name ?? '所选项目分摊' : '可见费用合计'}</h2><span className="muted">统计于 {date(report.as_of)}</span></div>{!report.totals.length ? <Empty title="本月尚无已确认支出" detail="可调整月份、类别或分摊范围，或前往台账核实待确认费用。" /> : <div className="cost-report-grid">{report.totals.map(total => { const drill = parameters(applied); drill.set('currency', total.currency); drill.set('state', 'incurred'); return <article className="panel cost-report-card" key={total.currency}><div className="eyebrow">{total.currency} / 原币</div><h3>发生支出净额</h3><p className="cost-net">{money(total.net_amount, total.currency)}</p><dl className="file-facts"><div><dt>原费用发生额</dt><dd>{money(total.expense_amount, total.currency)}</dd></div><div><dt>冲销减少额</dt><dd>{money(total.reversal_amount, total.currency)}</dd></div><div><dt>记录数量</dt><dd>{total.entry_count}</dd></div></dl><Link className="text-button" to={'/costs?' + drill}>核对费用明细<ArrowUpRight size={16} /></Link></article>; })}</div>}<p className="notice warning">当前期间有 {report.unconfirmed_count} 笔可见草稿尚未计入。此数量按已填写的发生日及分摊筛选，未填日期的草稿请到费用台账核对。</p></>}
  </>;
}
