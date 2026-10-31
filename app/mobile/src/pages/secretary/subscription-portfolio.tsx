import { Text, View } from '@tarojs/components';
import { useDidHide } from '@tarojs/taro';
import { useEffect, useState } from 'react';
import { checkedPortfolio, portfolioState, portfolioTextPage, portfolioTextPages, portfolioWindow, type PortfolioPreview } from '@copypaste/contracts/subscription-portfolio';
import { Scope, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Fact, Notice, Section } from '../../ui';
import { TextField } from '../business/controls';

export function ConversationSubscriptionPortfolio({ conversation: c }: { conversation: Schemas['Conversation'] }) {
  const [from, setFrom] = useState(''), [to, setTo] = useState(''), [request, setRequest] = useState<{ from: string; to: string; n: number }>(), [value, setValue] = useState<PortfolioPreview>(), [error, setError] = useState<unknown>(), [busy, setBusy] = useState(false), [open, setOpen] = useState(false), [page, setPage] = useState(0);
  const clear = () => { setRequest(undefined); setValue(undefined); setError(undefined); setOpen(false); setPage(0); };
  useDidHide(clear);
  useEffect(() => {
    const scope = new Scope(); setValue(undefined); setError(undefined); setBusy(!!request);
    if (request) void client.request<PortfolioPreview>('/conversations/' + c.id + '/subscription-review?from=' + request.from + '&to=' + request.to, { scope }).then(v => { const out = checkedPortfolio(v, c, request.from, request.to); if (!scope.closed) setValue(out); }).catch(e => { if (!scope.closed) setError(e); }).finally(() => { if (!scope.closed) setBusy(false); });
    return () => scope.close();
  }, [c.id, c.current_revision_id, c.context_security_version, request]);
  if (c.type === 'telegram' || c.state !== 'active') return null;
  return <Section title='跨订阅完整审阅'><View className='cp-pad'><Text className='cp-paragraph'>读取当前会话范围内、本人可读且允许秘书处理的全部订阅。公司会话仅含公司订阅，金额不跨币种合计。</Text>
    <TextField name='portfolio-from' label='审阅开始日（YYYY-MM-DD）' value={from} onChange={v => { clear(); setFrom(v); }} disabled={false} max={10} />
    <TextField name='portfolio-to' label='审阅结束日，不含（YYYY-MM-DD）' value={to} onChange={v => { clear(); setTo(v); }} disabled={false} max={10} />
    <Text className='cp-paragraph'>按订阅所在地的账期开始日选择，最多366日。日程覆盖不代表付清或供应商停费。</Text>
    <Action secondary disabled={!portfolioWindow(from, to) || busy} onClick={() => { setOpen(false); setPage(0); setRequest(v => ({ from, to, n: (v?.n ?? 0) + 1 })); }}>读取完整审阅</Action><Notice error={error} />{busy && <Busy />}
    {!busy && !error && value && <><Fact label='读取时间'>{value.captured_at}</Fact><Fact label='全部可读订阅'>{value.summary.subscription_count}个</Fact><Fact label='账期覆盖'>{value.summary.stored_period_count}个已存 · {value.summary.missing_period_count}个缺失 · {value.summary.schedule_coverage_known ? value.summary.schedule_coverage_complete ? '应有账期已全部生成' : '仍有缺失' : '部分日程尚不能确定'}</Fact>
      <Fact label='去重关联原记录'>费用{value.summary.unique_cost_count}笔 · 付款{value.summary.unique_payment_count}笔 · 共享{value.summary.shared_record_count}笔；共享本身不证明重复收费。</Fact>
      {value.summary.items.map(v => <View key={v.id} className='cp-inset'><Text className='cp-section-title'>{v.product} · {portfolioState[v.state] ?? v.state}</Text><Text selectable className='cp-code'>{v.id}</Text><Text className='cp-paragraph'>已存{v.stored_period_count}期 · 应有{v.coverage_known ? v.expected_period_count : '待明确'}期 · 缺失{v.coverage_known ? v.missing_period_count : '待明确'}期</Text></View>)}
      <Text className='cp-paragraph'>{value.coverage_notice}</Text><Text className='cp-paragraph'>此处为只读预览；可在秘书请求中明确选择订阅审阅并说明日期。实际工具回执才会保存为可引用快照。</Text>
      <Action secondary onClick={() => { setOpen(v => !v); setPage(0); }}>{open ? '收起逐笔依据' : '查看完整逐笔依据'}</Action>
      {open && <View className='cp-inset'><Text>同次完整内容 · 第{page + 1}／{portfolioTextPages(value)}页</Text><Text selectable className='cp-code'>{value.content_sha256}</Text><Text selectable className='cp-pre'>{portfolioTextPage(value, page)}</Text><Action secondary disabled={page === 0} onClick={() => setPage(v => v - 1)}>上一页依据</Action><Action secondary disabled={page + 1 >= portfolioTextPages(value)} onClick={() => setPage(v => v + 1)}>下一页依据</Action></View>}
    </>}
  </View></Section>;
}
