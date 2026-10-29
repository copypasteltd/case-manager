import { useEffect, useRef, useState } from 'react';
import { Text, View } from '@tarojs/components';
import { amortizationInput, amortizationMethods, checkedAmortizationOverview, checkedAmortizationPreview, checkedAmortizationVersion, type AmortizationContent, type AmortizationOverview, type AmortizationPreview, type AmortizationVersion } from '@copypaste/contracts/amortization';
import { formatReportAmount } from '@copypaste/contracts/finance-reports';
import { Input } from '../../platform/components';
import { MFAConfirmation } from '../../platform/command';
import { MobileError, Scope, type Page as ResultPage, type Schemas } from '../../platform/session';
import { client } from '../../platform/taro';
import { Action, Busy, Fact, Field, Notice, Section } from '../../ui';

export const amortizationViewNotice = '摊销是独立的费用分期视图。发布新版本或停用会重列当前计划口径，历史版本与已完成导出保留；原费用、审批和现金事实不变。';
function useResource<T>(path:string|null,parent:Scope,revision:number,validate:(v:T)=>T) {
  const [value,setValue]=useState<T>(),[error,setError]=useState<unknown>(),[loading,setLoading]=useState(false),check=useRef(validate);check.current=validate;
  useEffect(()=>{const scope=new Scope(),unbind=parent.own(()=>scope.close());setValue(undefined);setError(undefined);setLoading(Boolean(path));if(path)void client.request<T>(path,{scope}).then(v=>{if(!scope.closed)setValue(check.current(v));}).catch(e=>{if(!scope.closed)setError(e);}).finally(()=>{if(!scope.closed)setLoading(false);});return()=>{scope.close();unbind();};},[path,parent,revision]);
  return {value,error,loading};
}
export function CostAmortization({cost,scope}:{cost:Schemas['Cost'];scope:Scope}) {
  const [open,setOpen]=useState(false);
  if(cost.entry_kind!=='expense')return null;
  return <Section title="费用摊销计划"><Text className="cp-muted cp-paragraph">{amortizationViewNotice}</Text><Action secondary onClick={()=>setOpen(!open)}>{open?'收起摊销计划':'查阅摊销计划'}</Action>{open&&<PlanBody key={cost.id+':'+cost.version} cost={cost} scope={scope}/>}</Section>;
}
function PlanBody({cost,scope}:{cost:Schemas['Cost'];scope:Scope}) {
  const path='/costs/'+cost.id+'/amortization',[revision,setRevision]=useState(0),[cursors,setCursors]=useState(['']),[detail,setDetail]=useState<string>(),[editing,setEditing]=useState(false);
  const overview=useResource<AmortizationOverview>(path,scope,revision,v=>checkedAmortizationOverview(v,cost));
  const history=useResource<ResultPage<AmortizationVersion>>(path+'/versions?limit=10'+(cursors.at(-1)?'&cursor='+encodeURIComponent(cursors.at(-1)!):''),scope,revision,v=>{
    if(!v||!Array.isArray(v.items)||v.items.length>10||v.next_cursor!==null&&typeof v.next_cursor!=='string')throw new Error('摊销版本列表无效');v.items.forEach(item=>checkedAmortizationVersion(item,cost));return v;
  });
  const version=useResource<AmortizationVersion>(detail?path+'/versions/'+detail:null,scope,0,v=>{checkedAmortizationVersion(v,cost);if(v.id!==detail||!v.content)throw new Error('历史版本依据不完整');return v;});
  const refresh=()=>{setEditing(false);setCursors(['']);setDetail(undefined);setRevision(v=>v+1);},value=overview.value;
  return <View><Notice error={overview.error??history.error}/>{overview.loading?<Busy/>:value&&<>
    <Fact label="当前计划"><Text>{value.current?'第 '+value.version+' 版 · '+(value.current.state==='active'?'有效':'已停用'):'未发布，未自动采用任何规则'}</Text></Fact>
    {value.current&&<Text className="cp-muted cp-paragraph">发布依据：{value.current.reason} · {value.current.created_at}</Text>}
    {value.current?.state==='active'&&value.current.content&&<Schedule value={value.current.content}/>}
    {value.can_manage?<><Action secondary onClick={()=>setEditing(!editing)}>{editing?'关闭计划操作':'编制或停用摊销计划'}</Action>{editing&&<PlanEditor key={value.version} cost={cost} overview={value} parent={scope} refresh={refresh}/>}</>:<Text className="cp-muted cp-paragraph">当前仅查阅。发布需已确认原费用、可写项目范围及费用摊销管理权限。</Text>}
  </>}<Action secondary onClick={refresh}>刷新计划与历史</Action>
    <Text className="cp-section-title">不可修改的计划版本</Text>{history.value?.items.map(v=><View key={v.id}><Text className="cp-paragraph">第 {v.version} 版 · {v.state==='active'?'发布':'停用'} · {v.period_count} 个月 · {v.reason}</Text><Action secondary onClick={()=>setDetail(v.id)}>查看第 {v.version} 版依据</Action></View>)}
    {cursors.length>1&&<Action secondary onClick={()=>setCursors(v=>v.slice(0,-1))}>上一页摊销版本</Action>}{history.value?.next_cursor&&<Action secondary onClick={()=>setCursors(v=>[...v,history.value!.next_cursor!])}>更多摊销版本</Action>}
    {detail&&<Section title="摊销版本依据"><Notice error={version.error}/>{version.loading?<Busy/>:version.value&&<><Text className="cp-paragraph">第 {version.value.version} 版 · {version.value.reason}</Text><Text className="cp-small cp-code" selectable>操作者 {version.value.actor_id} · {version.value.created_at}</Text><Text className="cp-small cp-code" selectable>SHA-256：{version.value.content_sha256}</Text>{version.value.content&&<Schedule value={version.value.content}/>}</>}<Action secondary onClick={()=>setDetail(undefined)}>关闭版本依据</Action></Section>}
  </View>;
}
function PlanEditor({cost,overview,parent,refresh}:{cost:Schemas['Cost'];overview:AmortizationOverview;parent:Scope;refresh():void}) {
  const [method,setMethod]=useState(''),[from,setFrom]=useState(''),[to,setTo]=useState(''),[reason,setReason]=useState(''),[preview,setPreview]=useState<AmortizationPreview>(),[ack,setAck]=useState(false),[fresh,setFresh]=useState(false),[busy,setBusy]=useState(false),[uncertain,setUncertain]=useState(false),[blocked,setBlocked]=useState(false),[operation,setOperation]=useState(''),[error,setError]=useState<unknown>();
  const scope=useRef(new Scope()),flight=useRef(false),command=useRef({signature:'',key:''});
  useEffect(()=>{const current=new Scope();scope.current=current;const unbind=parent.own(()=>current.close());return()=>{current.close();unbind();};},[parent]);
  const path='/costs/'+cost.id+'/amortization',locked=busy||uncertain||blocked;
  const changed=()=>{setPreview(undefined);setAck(false);setError(undefined);};
  function input(){try{return amortizationInput(cost,overview.version,method,from,to,reason);}catch(e){throw new MobileError('AMORTIZATION_FORM_INVALID',e instanceof Error?e.message:'请核对摊销日期和规则',422);}}
  async function submit(kind:string){
    if(flight.current||scope.current.closed)return;flight.current=true;setOperation(kind);setBusy(true);setError(undefined);
    try{
      if(kind!=='preview'&&(!ack||!fresh||!reason.trim()))throw new MobileError('AMORTIZATION_REVIEW_REQUIRED','请核对口径、填写依据并完成两步验证。',422);
      if(kind==='publish'&&(!preview||cost.state!=='confirmed'))throw new MobileError('AMORTIZATION_PREVIEW_REQUIRED','请先预览已确认且未冲销的原费用。',422);
      const body=kind==='preview'?input():kind==='publish'?{...input(),preview_sha256:preview!.content_sha256,acknowledged:true as const}:{expected_version:cost.version,revision_id:cost.current_revision_id,content_hash:cost.content_hash,reason:reason.trim(),expected_plan_version:overview.version,acknowledged:true as const};
      const signature=JSON.stringify({kind,body});if(command.current.signature!==signature)command.current={signature,key:client.operationKey()};
      const result=await client.request<AmortizationPreview|AmortizationVersion>(path+(kind==='preview'?'/preview':kind==='stop'?'/stop':''),{method:'POST',body,key:command.current.key,scope:scope.current});
      if(scope.current.closed)return;
      if(kind==='preview'){setPreview(checkedAmortizationPreview(result as AmortizationPreview,cost,body as ReturnType<typeof input>));setAck(false);}
      else{const v=checkedAmortizationVersion(result as AmortizationVersion,cost,overview.version+1);if(!v.content||v.state!==(kind==='stop'?'stopped':'active')||v.content_sha256!==(kind==='stop'?overview.current?.content_sha256:preview?.content_sha256))throw new Error('计划操作回执尚未核对，请刷新历史。');refresh();}
      setUncertain(false);
    }catch(e){if(!scope.current.closed){setError(e instanceof Error?new MobileError('AMORTIZATION_FAILED',e.message,e instanceof MobileError?e.status:0):e);if(e instanceof MobileError&&[403,404,409,412].includes(e.status)){setBlocked(true);setPreview(undefined);}else if(!(e instanceof MobileError)||e.status===0||e.status>=500)setUncertain(true);}}
    finally{flight.current=false;if(!scope.current.closed)setBusy(false);}
  }
  return <Section title={cost.state==='confirmed'?'人工编制新版本':'费用已冲销，仅可停用计划'}><Notice error={error}/><Text className="cp-muted cp-paragraph">月均法将每个相交月份等权处理；按日法按实际服务天数分配。每个原项目及公司份额分别累计四舍五入，最多 120 个月。</Text>
    {cost.state==='confirmed'&&<>{amortizationMethods.map(v=><Action key={v.value} secondary={method!==v.value} disabled={locked} onClick={()=>{setMethod(v.value);changed();}}>{v.label}{method===v.value?' · 已选择':''}</Action>)}<Field label="服务起始日 YYYY-MM-DD"><Input ariaLabel="服务起始日" className="cp-input" maxlength={10} value={from} disabled={locked} onInput={e=>{setFrom(e.detail.value);changed();}}/></Field><Field label="服务结束日 YYYY-MM-DD（不含）"><Input ariaLabel="服务结束日（不含）" className="cp-input" maxlength={10} value={to} disabled={locked} onInput={e=>{setTo(e.detail.value);changed();}}/></Field></>}
    <Field label="计划操作依据"><Input ariaLabel="计划操作依据" className="cp-input" maxlength={200} value={reason} disabled={locked} onInput={e=>{setReason(e.detail.value);changed();}}/></Field>
    {cost.state==='confirmed'&&<Action secondary disabled={locked||!method||!from||!to||!reason.trim()} onClick={()=>void submit('preview')}>预览月度摊销</Action>}
    {preview&&<><Text className="cp-section-title">待发布预览 · 第 {preview.plan_version} 版</Text><Schedule value={preview.content}/><Text className="cp-small cp-code" selectable>SHA-256：{preview.content_sha256}</Text></>}
    {(preview||overview.current?.state==='active')&&<><MFAConfirmation disabled={locked} onFresh={setFresh}/><Action secondary={!ack} disabled={locked} onClick={()=>setAck(!ack)}>{ack?'已确认重列当前计划口径并保留历史':'确认重列当前计划口径并保留历史'}</Action>
    {preview&&<Action disabled={locked||!ack||!fresh} onClick={()=>void submit('publish')}>确认发布摊销计划</Action>}{overview.current?.state==='active'&&<Action secondary disabled={locked||!ack||!fresh||!reason.trim()} onClick={()=>void submit('stop')}>确认停用当前计划</Action>}</>}
    {busy&&<Busy/>}{blocked&&<Text className="cp-notice">费用、权限或计划已变化。请刷新计划和原费用，重新核对。</Text>}{uncertain&&<><Text className="cp-notice">结果尚未确认，输入已锁定；可原样重试核对同一回执，或刷新计划历史。</Text><Action secondary disabled={busy} onClick={()=>void submit(operation)}>原样重试计划操作</Action></>}
  </Section>;
}
function Schedule({value}:{value:AmortizationContent}) {
  const [selected,setSelected]=useState<number>();
  return <View><Text className="cp-paragraph">{amortizationMethods.find(v=>v.value===value.method)?.label} · {value.currency} {formatReportAmount(value.amount)} · {value.service_from} 至 {value.service_to}（不含）</Text>
    {value.periods.map(p=><View className="cp-amortization-period" key={p.sequence}><Text className="cp-paragraph">第 {p.sequence} 期 · {p.recognized_on} · {value.currency} {formatReportAmount(p.amount)}</Text><Text className="cp-muted cp-small">{p.service_from} → {p.service_to}（不含）</Text>
      {p.shares.length?<><Action secondary onClick={()=>setSelected(selected===p.sequence?undefined:p.sequence)}>第 {p.sequence} 期 · {p.shares.length} 项分摊依据</Action>{selected===p.sequence&&p.shares.map(s=><Text className="cp-small cp-paragraph" key={s.project_id??'company'} selectable>{s.project_id?'项目 '+s.project_id:'公司'}：{formatReportAmount(s.amount)}</Text>)}</>:<Text className="cp-muted cp-small">零金额月份</Text>}
    </View>)}</View>;
}
