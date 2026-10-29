import { useEffect, useRef, useState } from 'react';
import { useInfiniteQuery, useMutation, useQuery } from '@tanstack/react-query';
import { amortizationInput, amortizationMethods, checkedAmortizationOverview, checkedAmortizationPreview, checkedAmortizationVersion, type AmortizationContent, type AmortizationOverview, type AmortizationPreview, type AmortizationVersion } from '@copypaste/contracts/amortization';
import { formatReportAmount } from '@copypaste/contracts/finance-reports';
import { api, ApiError, type Me, type Page } from './api';
import { Busy, Dialog, ErrorNotice, Field } from './components';
import { useCommandKey } from './BusinessSelectors';
import { MFAConfirmation } from './MFAConfirmation';
import type { Cost } from './CostDisplay';

export const amortizationViewNotice = '摊销是独立的费用分期视图。发布新版本或停用会重列当前计划口径，历史版本与已完成导出保留；原费用、审批和现金事实不变。';
export function CostAmortization({ cost, me }: { cost: Cost; me: Me }) {
  const [open, setOpen] = useState(false);
  if (cost.entry_kind !== 'expense') return null;
  return <section className="panel report-sources"><div className="section-heading"><div><div className="eyebrow">SERVICE PERIOD / ALLOCATION SCHEDULE</div><h2>费用摊销计划</h2></div><button className="button secondary" onClick={() => setOpen(!open)} aria-expanded={open}>查阅摊销计划</button></div><p className="muted">{amortizationViewNotice}</p>{open && <AmortizationBody key={[cost.id,cost.version,me.authorization_version].join(':')} cost={cost} />}</section>;
}
function AmortizationBody({ cost }: { cost: Cost }) {
  const path = '/costs/' + cost.id + '/amortization', [revision, setRevision] = useState(0), [detail,setDetail] = useState<string>(), [editing,setEditing] = useState(false);
  const overview = useQuery({ queryKey: ['cost-amortization',cost.id,cost.version,revision], queryFn: async ({signal}) => checkedAmortizationOverview(await api<AmortizationOverview>(path,{signal}),cost), refetchOnWindowFocus: false });
  const history = useInfiniteQuery({ queryKey: ['cost-amortization-history',cost.id,revision], initialPageParam: '', queryFn: async ({pageParam,signal}) => {
    const page = await api<Page<AmortizationVersion>>(path + '/versions?limit=10' + (pageParam ? '&cursor=' + encodeURIComponent(pageParam) : ''),{signal});
    if (!page || !Array.isArray(page.items) || page.items.length>10 || page.next_cursor !== null && typeof page.next_cursor !== 'string') throw new Error('摊销版本列表无效');
    page.items.forEach(v => checkedAmortizationVersion(v,cost)); return page;
  }, getNextPageParam: v => v.next_cursor ?? undefined, refetchOnWindowFocus: false });
  const version = useQuery({ queryKey: ['cost-amortization-version',cost.id,detail], enabled: Boolean(detail), queryFn: async ({signal}) => {
    const v = checkedAmortizationVersion(await api<AmortizationVersion>(path + '/versions/' + detail,{signal}),cost);
    if (v.id !== detail || !v.content) throw new Error('历史版本依据不完整'); return v;
  } });
  const value = overview.error ? undefined : overview.data;
  return <><ErrorNotice error={overview.error ?? history.error} />{overview.isPending ? <Busy label="读取费用摊销计划" /> : value && <>
    <p>当前：{value.current ? '第 ' + value.version + ' 版 · ' + (value.current.state === 'active' ? '有效' : '已停用') : '未发布，未自动采用任何规则'}</p>
    {value.current && <><p className="muted">发布依据：{value.current.reason} · {value.current.created_at}</p>{value.current.state === 'active' && value.current.content && <AmortizationSchedule value={value.current.content} />}</>}
    {value.can_manage && <><button className="button secondary" onClick={()=>setEditing(!editing)}>{editing?'关闭计划操作':'编制或停用摊销计划'}</button>{editing && <AmortizationEditor key={value.version} cost={cost} value={value} refresh={() => { setEditing(false); setRevision(v=>v+1); }} />}</>}
    {!value.can_manage && <p className="muted">当前仅查阅。发布需已确认原费用、可写项目范围及费用摊销管理权限。</p>}
  </>}<button className="text-button" onClick={() => { setDetail(undefined); setEditing(false); setRevision(v=>v+1); }}>刷新计划与历史</button>
    <h3>不可修改的计划版本</h3>{!history.error && history.data?.pages.flatMap(v=>v.items).map(v => <div className="report-context" key={v.id}><span>第 {v.version} 版 · {v.state === 'active' ? '发布' : '停用'} · {v.period_count} 个月 · {v.reason}</span><button className="text-button" onClick={()=>setDetail(v.id)}>查看第 {v.version} 版依据</button></div>)}
    {history.hasNextPage && <button className="button secondary" disabled={history.isFetchingNextPage} onClick={()=>void history.fetchNextPage()}>更多摊销版本</button>}
    {detail && <Dialog title="摊销版本依据" onClose={()=>setDetail(undefined)}><ErrorNotice error={version.error}/>{version.isPending ? <Busy/> : version.data && <><p>第 {version.data.version} 版 · {version.data.state === 'active' ? '发布' : '停用'} · {version.data.reason}</p><p className="muted">操作者 {version.data.actor_id} · {version.data.created_at}</p><p className="record-code">SHA-256：{version.data.content_sha256}</p>{version.data.content && <AmortizationSchedule value={version.data.content}/>}</>}</Dialog>}
  </>;
}
function AmortizationEditor({cost,value,refresh}: {cost:Cost;value:AmortizationOverview;refresh():void}) {
  const [method,setMethod] = useState(''), [from,setFrom] = useState(''), [to,setTo] = useState(''), [reason,setReason] = useState(''), [preview,setPreview] = useState<AmortizationPreview>(), [ack,setAck] = useState(false), [fresh,setFresh] = useState(false), [uncertain,setUncertain] = useState(false), [blocked,setBlocked] = useState(false), [operation,setOperation] = useState('');
  const key=useCommandKey(), lifetime=useRef(new AbortController()), path='/costs/'+cost.id+'/amortization';
  useEffect(()=>{const current=new AbortController();lifetime.current=current;return()=>current.abort();},[]);
  const inputForPreview=()=>{try{return amortizationInput(cost,value.version,method,from,to,reason);}catch(e){throw new ApiError(422,'AMORTIZATION_FORM_INVALID',e instanceof Error?e.message:'请核对摊销规则和日期。','');}};
  const command=useMutation({mutationFn:async(kind:string)=>{
    if (kind==='preview') {
      const input=inputForPreview();
      return {kind,preview:checkedAmortizationPreview(await api<AmortizationPreview>(path+'/preview',{method:'POST',body:input,key:key({kind,input}),signal:lifetime.current.signal}),cost,input)};
    }
    if (!ack || !fresh || !reason.trim()) throw new ApiError(422,'AMORTIZATION_REVIEW_REQUIRED','请核对口径、填写依据并完成两步验证。','');
    if(kind==='publish' && (!preview || cost.state!=='confirmed')) throw new Error('请先预览已确认、未冲销费用的计划。');
    const input=kind==='publish' ? {...inputForPreview(),preview_sha256:preview!.content_sha256,acknowledged:true as const} :
      {expected_version:cost.version,revision_id:cost.current_revision_id,content_hash:cost.content_hash,reason:reason.trim(),expected_plan_version:value.version,acknowledged:true as const};
    const result=checkedAmortizationVersion(await api<AmortizationVersion>(path+(kind==='stop'?'/stop':''),{method:'POST',body:input,key:key({kind,input}),signal:lifetime.current.signal}),cost,value.version+1);
    if (!result.content || result.state!==(kind==='stop'?'stopped':'active') || result.content_sha256!==(kind==='stop'?value.current?.content_sha256:preview?.content_sha256)) throw new Error('发布回执尚未核对，请刷新计划与历史。');
    return {kind};
  }, onSuccess:result=>{setUncertain(false);if(result.kind==='preview'){setPreview(result.preview);setAck(false);}else refresh();},
    onError:error=>{if(error instanceof ApiError && [403,404,409,412].includes(error.status)){setBlocked(true);setPreview(undefined);}else if(!(error instanceof ApiError)||error.status===0||error.status>=500){setUncertain(true);}}});
  const locked=command.isPending||uncertain||blocked;
  const changed=()=>{setPreview(undefined);setAck(false);command.reset();};
  function submit(kind:string){setOperation(kind);command.mutate(kind);}
  return <div className="amortization-editor"><h3>{cost.state==='confirmed'?'人工编制新版本':'当前费用已冲销，仅可停用计划'}</h3>
    <p className="muted">月均法将每个相交月份等权处理；按日法按实际服务天数分配。每个原始项目及公司份额分别累计四舍五入，保留原币精度。服务结束日不含在内，最多 120 个月。</p>
    <ErrorNotice error={command.error}/>{cost.state==='confirmed' && <div className="party-filters">
      <Field label="摊销规则"><select aria-label="摊销规则" value={method} disabled={locked} onChange={e=>{setMethod(e.target.value);changed();}}><option value="">请明确选择</option>{amortizationMethods.map(v=><option key={v.value} value={v.value}>{v.label}</option>)}</select></Field>
      <Field label="服务起始日"><input aria-label="服务起始日" type="date" value={from} disabled={locked} onChange={e=>{setFrom(e.target.value);changed();}}/></Field>
      <Field label="服务结束日（不含）"><input aria-label="服务结束日（不含）" type="date" value={to} disabled={locked} onChange={e=>{setTo(e.target.value);changed();}}/></Field>
    </div>}
    <Field label="计划操作依据"><input aria-label="计划操作依据" maxLength={200} value={reason} disabled={locked} onChange={e=>{setReason(e.target.value);changed();}}/></Field>
    {cost.state==='confirmed' && <button className="button secondary" disabled={locked||!method||!from||!to||!reason.trim()} onClick={()=>submit('preview')}>预览月度摊销</button>}
    {preview && <><h3>待发布预览 · 第 {preview.plan_version} 版</h3><AmortizationSchedule value={preview.content}/><p className="record-code">SHA-256：{preview.content_sha256}</p></>}
    {(preview||value.current?.state==='active') && <><MFAConfirmation onFresh={setFresh}/><label className="amortization-ack"><input type="checkbox" checked={ack} disabled={locked} onChange={e=>setAck(e.target.checked)}/>我确认该操作将重列当前摊销计划视图，历史版本及已完成导出保留</label><div className="form-actions">
      {preview && <button className="button" disabled={locked||!ack||!fresh} onClick={()=>submit('publish')}>确认发布摊销计划</button>}
      {value.current?.state==='active' && <button className="button secondary" disabled={locked||!ack||!fresh||!reason.trim()} onClick={()=>submit('stop')}>确认停用当前计划</button>}
    </div></>}
    {command.isPending && <Busy label={operation==='preview'?'计算月度分摊':'提交计划操作'}/>}
    {blocked && <p className="notice warning">费用、权限或计划已变化。请刷新计划与原费用，重新核对后再操作。</p>}
    {uncertain && <><p className="notice warning">结果尚未确认，输入已锁定。可原样重试核对同一回执，或刷新计划历史。</p><button className="button secondary" disabled={command.isPending} onClick={()=>submit(operation)}>原样重试计划操作</button></>}
  </div>;
}
export function AmortizationSchedule({value}: {value:AmortizationContent}) {
  return <div className="amortization-schedule"><p><strong>{amortizationMethods.find(v=>v.value===value.method)?.label} · {value.currency} {formatReportAmount(value.amount)}</strong> · {value.service_from} 至 {value.service_to}（不含）</p><div className="table-scroll"><table className="data-table"><caption>月度计划及原项目／公司份额</caption><thead><tr><th>期次</th><th>服务区间（结束不含）</th><th>确认日期</th><th>计划金额 {value.currency}</th><th>份额依据</th></tr></thead><tbody>{value.periods.map(p=><tr key={p.sequence}><td>{p.sequence}</td><td>{p.service_from} → {p.service_to}</td><td>{p.recognized_on}</td><td>{formatReportAmount(p.amount)}</td><td>{p.shares.length ? <details><summary>{p.shares.length} 项分摊</summary>{p.shares.map(s=><p key={s.project_id??'company'}>{s.project_id?'项目 '+s.project_id:'公司'}：{formatReportAmount(s.amount)}</p>)}</details>:'零金额月份'}</td></tr>)}</tbody></table></div></div>;
}
