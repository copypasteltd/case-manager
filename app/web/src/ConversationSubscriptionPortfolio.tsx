import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { checkedPortfolio, portfolioState, portfolioTextPage, portfolioTextPages, portfolioWindow, type PortfolioPreview } from '@copypaste/contracts/subscription-portfolio';
import { api, type Me, type Schemas } from './api';
import { Busy, date, ErrorNotice, Field } from './components';

export function ConversationSubscriptionPortfolio({ conversation: c, me }: { conversation: Schemas['Conversation']; me: Me }) {
  const [from, setFrom] = useState(''), [to, setTo] = useState(''), [request, setRequest] = useState<{ from: string; to: string; n: number }>(), [open, setOpen] = useState(false), [page, setPage] = useState(0);
  const preview = useQuery({ queryKey: ['subscription-portfolio', c.id, c.current_revision_id, c.context_security_version, me.authorization_version, request], enabled: !!request, gcTime: 0, retry: false,
    queryFn: async ({ signal }) => { const r = request!; return checkedPortfolio(await api<PortfolioPreview>('/conversations/' + c.id + '/subscription-review?' + new URLSearchParams({ from: r.from, to: r.to }), { signal }), c, r.from, r.to); } });
  if (c.type === 'telegram' || c.state !== 'active' || !me.actions.includes('report.read')) return null;
  const value = request && !preview.isFetching && !preview.error ? preview.data : undefined;
  const clear = () => { setRequest(undefined); setOpen(false); setPage(0); };
  return <section className="panel conversation-summary" data-testid="subscription-portfolio"><h2>跨订阅完整审阅</h2>
    <p className="form-note">查阅当前会话范围内、本人可读且允许秘书处理的全部订阅。公司会话仅包含公司订阅；不会合计不同币种。</p>
    <Field label="审阅开始日"><input type="date" value={from} onChange={e => { clear(); setFrom(e.target.value); }} /></Field>
    <Field label="审阅结束日（不含）"><input type="date" value={to} onChange={e => { clear(); setTo(e.target.value); }} /></Field>
    <p className="form-note">按订阅所在地的账期开始日选择，最多366日。日程覆盖完整不代表付清或供应商已停止收费。</p>
    <button type="button" className="button secondary" disabled={!portfolioWindow(from, to) || preview.isFetching} onClick={() => { setOpen(false); setPage(0); setRequest(v => ({ from, to, n: (v?.n ?? 0) + 1 })); }}>读取完整审阅</button>
    <ErrorNotice error={preview.error} />{preview.isFetching && <Busy label="正在核对全部订阅与逐笔来源" />}
    {value && <><p>读取时间：{date(value.captured_at)} · {value.summary.subscription_count}个订阅 · {value.summary.stored_period_count}个已存账期 · {value.summary.missing_period_count}个缺失账期</p>
      <p>{value.summary.schedule_coverage_known ? value.summary.schedule_coverage_complete ? '所选日期内应有账期已全部生成。' : '所选日期内存在尚未生成的应有账期。' : '部分日程尚不能确定；没有可读订阅也不能证明没有费用。'}</p>
      <p>去重关联费用 {value.summary.unique_cost_count}笔 · 付款 {value.summary.unique_payment_count}笔 · 跨订阅共享原记录 {value.summary.shared_record_count}笔。共享关联本身不代表重复收费。</p>
      <div className="table-scroll"><table><thead><tr><th>订阅</th><th>状态</th><th>已存账期</th><th>应有账期</th><th>缺失账期</th></tr></thead><tbody>{value.summary.items.map(v => <tr key={v.id}><td>{v.product}<code className="full-digest">{v.id}</code></td><td>{portfolioState[v.state] ?? v.state}</td><td>{v.stored_period_count}</td><td>{v.coverage_known ? v.expected_period_count : '待明确'}</td><td>{v.coverage_known ? v.missing_period_count : '待明确'}</td></tr>)}</tbody></table></div>
      <p className="form-note">{value.coverage_notice}</p><p className="form-note">此处为只读预览。可在下方明确选择“订阅审阅”并说明日期，请秘书使用完整审阅工具；实际工具回执才会保存为可引用快照。</p>
      <button type="button" className="button secondary" onClick={() => { setOpen(v => !v); setPage(0); }}>{open ? '收起逐笔依据' : '查看完整逐笔依据'}</button>
      {open && <div className="conversation-source-preview"><p>同次完整内容 · 第{page + 1}／{portfolioTextPages(value)}页</p><code className="full-digest">{value.content_sha256}</code><pre>{portfolioTextPage(value, page)}</pre><div className="actions"><button type="button" className="button secondary" disabled={page === 0} onClick={() => setPage(v => v - 1)}>上一页依据</button><button type="button" className="button secondary" disabled={page + 1 >= portfolioTextPages(value)} onClick={() => setPage(v => v + 1)}>下一页依据</button></div></div>}
    </>}
  </section>;
}
