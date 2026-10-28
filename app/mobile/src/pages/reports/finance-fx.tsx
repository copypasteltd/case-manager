import { useEffect, useRef, useState } from 'react';
import { Text, View } from '@tarojs/components';
import { checkedFXBook, checkedFXPolicy, fxContentFromText, fxProjectQuery, fxRatesText, type FXBook, type FXPolicy, type FXValuation } from '@copypaste/contracts/finance-fx';
import { formatReportAmount } from '@copypaste/contracts/finance-reports';
import { Input, Textarea } from '../../platform/components';
import { useCollection } from '../../platform/collection';
import { client } from '../../platform/taro';
import { MobileError, Scope, type Schemas } from '../../platform/session';
import { Action, Busy, Fact, Field, Notice, Section } from '../../ui';

function useFXResource<T>(path: string, parent: Scope, check: (v: T) => T) {
  const [value,setValue] = useState<T>(), [error,setError] = useState<unknown>(), [revision,setRevision] = useState(0);
  const validate = useRef(check); validate.current = check;
  useEffect(() => { const scope = new Scope(), unbind = parent.own(() => scope.close()); setValue(undefined); setError(undefined); void client.request<T>(path,{scope}).then(v => { if (!scope.closed) setValue(validate.current(v)); }).catch(e => { if (!scope.closed) setError(e); }); return () => { scope.close(); unbind(); }; },[path,parent,revision]);
  return { value,error,reload: () => setRevision(v => v + 1) };
}
export function FinanceFX({ me,scope,project,selected,choose }: { me: Schemas['Me']; scope: Scope; project?: string | null; selected?: string | null; choose(id: string | null): void }) {
  const [open,setOpen] = useState(false), [detail,setDetail] = useState<string>(), [create,setCreate] = useState(false);
  const context = fxProjectQuery(project), policy = useFXResource<FXPolicy>('/finance/fx-policy' + context,scope,checkedFXPolicy);
  const books = useCollection<FXBook>(open ? '/finance/fx-books' + context + (context ? '&' : '?') + 'limit=10' : null);
  return <Section title="基础币种与汇率"><Notice error={policy.error ?? books.error} /><Text className="cp-muted cp-paragraph">{selected ? '当前报表已固定汇率版本。' : '当前按原币分列，选择明确版本后显示折算。'}{policy.value?.current ? '最新第 ' + policy.value.version + ' 版 · ' + policy.value.current.base_currency : '公司尚未发布基础币种。'}</Text><Action secondary onClick={() => setOpen(!open)}>{open ? '收起汇率版本' : '选择汇率版本'}</Action>{open && <><Action secondary onClick={() => choose(null)}>仅原币统计</Action>{me.actions.includes('finance.fx.manage') && <Action onClick={() => setCreate(true)}>发布人工汇率版本</Action>}<Action secondary onClick={() => { policy.reload(); books.reload(); }}>刷新版本</Action>{books.loading ? <Busy /> : books.items.map(v => <Fact key={v.id} label={'第 ' + v.revision + ' 版 · ' + v.base_currency}><Text>{v.title} · {v.rate_count} 条汇率</Text><View className="cp-choice"><Action secondary onClick={() => setDetail(v.id)}>查阅依据</Action><Action secondary={selected !== v.id} onClick={() => { checkedFXBook(v); choose(v.id); }}>{selected === v.id ? '已选择' : '用于当前报表'}</Action></View></Fact>)}{books.next && <Action secondary onClick={() => void books.more()} loading={books.moreBusy}>更多汇率版本</Action>}</>}{detail && <FXDetail id={detail} project={project} scope={scope} close={() => setDetail(undefined)} />}{create && <FXPublish scope={scope} close={() => setCreate(false)} published={() => { setCreate(false); policy.reload(); books.reload(); }} />}</Section>;
}
function FXDetail({ id,project,scope,close }: { id: string; project?: string | null; scope: Scope; close(): void }) {
  const book = useFXResource<FXBook>('/finance/fx-books/' + id + fxProjectQuery(project),scope,v => checkedFXBook(v,true));
  return <Section title="汇率版本依据"><Notice error={book.error} />{book.value?.content ? <><Text>{book.value.title} · {book.value.base_currency} · 第 {book.value.revision} 版</Text><Text className="cp-muted cp-paragraph">{book.value.reason}</Text><Text selectable className="cp-small cp-paragraph">{fxRatesText(book.value.content)}</Text><Text selectable className="cp-small cp-paragraph">SHA-256：{book.value.content_sha256}</Text></> : !book.error && <Busy />}<Action secondary onClick={close}>关闭汇率依据</Action></Section>;
}
function FXPublish({ scope,close,published }: { scope: Scope; close(): void; published(): void }) {
  const policy = useFXResource<FXPolicy>('/finance/fx-policy',scope,checkedFXPolicy);
  const [title,setTitle] = useState(''), [base,setBase] = useState(''), [rates,setRates] = useState(''), [reason,setReason] = useState(''), [ack,setAck] = useState(false), [busy,setBusy] = useState(false), [uncertain,setUncertain] = useState(false), [error,setError] = useState<unknown>();
  const flight = useRef(false), command = useRef({signature:'',key:''});
  async function publish() {
    if (flight.current || scope.closed) return; flight.current = true; setBusy(true); setError(undefined);
    try { if (!policy.value || !ack || !reason.trim()) throw new MobileError('FX_FORM_INVALID','请填写依据并确认口径。',422); const body = { expected_version:policy.value.version,content:(() => { try { return fxContentFromText(title,base,rates); } catch(e) { throw new MobileError('FX_FORM_INVALID',e instanceof Error ? e.message : '人工汇率格式无效',422); } })(),reason:reason.trim(),acknowledged:true }; const signature = JSON.stringify(body); if (command.current.signature !== signature) command.current = { signature,key:client.operationKey() }; checkedFXBook(await client.request<FXBook>('/finance/fx-books',{ method:'POST',body,key:command.current.key,scope }),true); if (!scope.closed) published(); }
    catch (e) { if (!scope.closed) { setError(e instanceof Error ? new MobileError('FX_PUBLISH_FAILED',e.message,e instanceof MobileError ? e.status : 0) : e); if (!(e instanceof MobileError) || e.status === 0 || e.status >= 500) setUncertain(true); } }
    finally { flight.current = false; if (!scope.closed) setBusy(false); }
  }
  const locked = busy || uncertain;
  return <Section title="发布人工汇率版本"><Text className="cp-muted cp-paragraph">每条当前范围贡献额折算至基础币种精度后求和；不改写原币事实。报价为 1 单位原币对应多少基础币种，结束日不含，同币种区间不得重叠。</Text><Notice error={error ?? policy.error} /><Field label="版本标题"><Input ariaLabel="汇率版本标题" className="cp-input" maxlength={200} value={title} disabled={locked} onInput={e => setTitle(e.detail.value)} /></Field><Field label="基础币种"><Input ariaLabel="基础币种" className="cp-input" maxlength={3} value={base} disabled={locked} onInput={e => setBase(e.detail.value.toUpperCase())} /></Field><Field label="Tab 分隔：币种、起始日、结束日、报价日、汇率、来源"><Textarea ariaLabel="人工汇率明细" className="cp-textarea" maxlength={2097152} value={rates} disabled={locked} onInput={e => setRates(e.detail.value)} /></Field><Field label="发布依据"><Input ariaLabel="汇率发布依据" className="cp-input" maxlength={200} value={reason} disabled={locked} onInput={e => setReason(e.detail.value)} /></Field><Action secondary={!ack} disabled={locked} onClick={() => setAck(!ack)}>{ack ? '已确认基础币种、来源与舍入口径' : '确认基础币种、来源与舍入口径'}</Action>{uncertain && <Text className="cp-muted cp-paragraph">结果待确认，重试沿用同一请求标识。也可关闭后查阅已发布版本。</Text>}<Action loading={busy} disabled={!policy.value || !ack} onClick={() => void publish()}>{uncertain ? '重试并核对回执' : '确认发布'}</Action><Action secondary disabled={busy} onClick={close}>关闭发布表单</Action></Section>;
}
export function FXSummary({ value }: { value?: FXValuation }) {
  if (!value) return null;
  return <Section title={'基础币种折算 · ' + value.book.base_currency + ' · 第 ' + value.book.revision + ' 版'}><Text className="cp-metric-number">{value.total_amount === null ? '完整合计待补齐' : formatReportAmount(value.total_amount)}</Text><Text className="cp-paragraph">已覆盖 {formatReportAmount(value.covered_amount)} {value.book.base_currency} · {value.covered_count} 笔；缺汇率 {value.missing_rate_count} 笔 · 未定价 {value.unpriced_count} 笔 · 草稿未计入 {value.unconfirmed_count} 笔</Text>{value.currencies.filter(v => v.missing_rate_count || v.unpriced_count).map(v => <Text key={v.currency ?? 'unknown'} className="cp-paragraph">{v.currency ?? '未定币种'} · 缺汇率净额 {formatReportAmount(v.missing_native_amount)} · 缺口 {v.missing_rate_count + v.unpriced_count} 笔</Text>)}<Text className="cp-small cp-muted">{value.book.title} · 人工区间报价，逐条范围金额舍入后求和；冲销沿用原记录报价日，分摊范围之间可能有舍入尾差。</Text></Section>;
}
