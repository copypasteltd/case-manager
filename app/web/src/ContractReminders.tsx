import { useRef, useState } from 'react';
import { useInfiniteQuery, useMutation, useQuery } from '@tanstack/react-query';
import { checkedContractReminderPolicy, checkedContractReminderReceipt, checkedContractReminderState, contractReminderInput, type ContractReminderPolicy, type ContractReminderState } from '@copypaste/contracts/notifications';
import { api, ApiError, type Me, type Page } from './api';
import { Busy, date, ErrorNotice, Field } from './components';
import { MFAConfirmation } from './MFAConfirmation';

export default function ContractReminders({me}:{me:Me}) {
  const [editing,setEditing]=useState(false),[epoch,setEpoch]=useState(0);
  const policy=useQuery({queryKey:['contract-reminder-policy',me.user.id,me.authorization_version],queryFn:async({signal})=>checkedContractReminderState(await api<ContractReminderState>('/notification-contract-policy',{signal})),gcTime:0});
  const history=useInfiniteQuery({queryKey:['contract-reminder-history',me.user.id,me.authorization_version],initialPageParam:'',queryFn:async({pageParam,signal})=>{
    const value=await api<Page<ContractReminderPolicy>>('/notification-contract-policy-history?limit=10'+(pageParam?'&cursor='+encodeURIComponent(pageParam):''),{signal});value.items.forEach(checkedContractReminderPolicy);return value;
  },getNextPageParam:p=>p.next_cursor??undefined,gcTime:0});
  async function refresh(){setEditing(false);await Promise.all([policy.refetch(),history.refetch()]);setEpoch(v=>v+1);}
  return <section className="panel conversation-summary" aria-label="合同到期提醒策略"><div className="section-heading"><h2>合同到期提醒策略</h2><button className="button secondary" onClick={()=>void refresh()} disabled={policy.isFetching||history.isFetching}>刷新提醒策略</button></div>
    <p className="form-note">发布后按业务时区及提前量检查生效合同，通知当前有权责任人。新策略不补发早于发布业务日的提醒，已入队事件继续按原事实处理；停用不撤回既有通知。</p>
    <ErrorNotice error={policy.error??history.error}/>{policy.isPending?<Busy/>:!policy.error&&policy.data&&<>{policy.data.current?<PolicyView value={policy.data.current}/>:<p>尚未发布组织策略。若 Worker 已明确配置部署级提醒规则，该规则仍可能生效；发布后以组织策略为准。</p>}
      {policy.data.can_publish&&<button className="button" disabled={editing} onClick={()=>setEditing(true)}>修改提醒策略</button>}
      {editing&&policy.data.can_publish&&<PolicyForm key={(policy.data.current?.id??'initial')+':'+epoch} state={policy.data} me={me} done={()=>void refresh()} cancel={()=>setEditing(false)}/>}
    </>}
    <h3>策略发布历史</h3>{history.isPending?<Busy/>:!history.error&&<>{history.data?.pages.flatMap(p=>p.items).map(p=><article className="conversation-source-preview" key={p.id} data-policy-version={p.version}><PolicyView value={p}/><p className="muted">{p.reason} · 经办人 {p.created_by}</p></article>)}{!history.data?.pages.some(p=>p.items.length)&&<p className="muted">暂无已发布策略。</p>}{history.hasNextPage&&<button className="button secondary" disabled={history.isFetchingNextPage} onClick={()=>void history.fetchNextPage()}>加载更早策略</button>}</>}
  </section>;
}
function PolicyView({value:p}:{value:ContractReminderPolicy}){return <div><strong>版本 {p.version} · {p.enabled?'已启用':'已停用'}</strong><p>{p.timezone} · 提前 {p.days_before.length?p.days_before.join('、'):'无'} 天</p><p className="form-note">发布业务日 {p.effective_on} · {date(p.created_at)}</p></div>;}
function PolicyForm({state,me,done,cancel}:{state:ContractReminderState;me:Me;done():void;cancel():void}) {
  const p=state.current;
  const [enabled,setEnabled]=useState(p?.enabled??false),[timezone,setTimezone]=useState(p?.timezone??''),[days,setDays]=useState(p?.days_before.join(', ')??''),[reason,setReason]=useState(''),[confirmed,setConfirmed]=useState(false),[fresh,setFresh]=useState(false),[blocked,setBlocked]=useState(false);
  const pending=useRef<{body:string;key:string}>();
  const publish=useMutation({mutationFn:async()=>{
    const input=contractReminderInput(state,enabled,timezone,days,reason),encoded=JSON.stringify(input);
    if(!pending.current||pending.current.body!==encoded)pending.current={body:encoded,key:crypto.randomUUID()};
    let returned=false;
    try{const receipt=await api<ContractReminderPolicy>('/notification-contract-policy',{method:'POST',body:input,key:pending.current.key});returned=true;return checkedContractReminderReceipt(receipt,input,me.user.id);}
    catch(cause){if(returned||!(cause instanceof ApiError)||cause.status===0||cause.status>=500||[403,404,409,412].includes(cause.status))setBlocked(true);throw cause;}
  },onSuccess:done});
  const locked=publish.isPending||blocked;
  return <form aria-label="发布合同提醒策略" onSubmit={e=>{e.preventDefault();if(fresh&&confirmed&&!locked)publish.mutate();}}><p>基于版本 {p?.version??0}，发布新的不可变版本。</p><Field label="提醒开关"><select value={enabled?'enabled':'disabled'} disabled={locked} onChange={e=>{setConfirmed(false);setEnabled(e.target.value==='enabled');}}><option value="disabled">停用</option><option value="enabled">启用</option></select></Field><Field label="业务时区"><input required value={timezone} placeholder="Asia/Hong_Kong" maxLength={200} disabled={locked} onChange={e=>{setConfirmed(false);setTimezone(e.target.value);}}/></Field><Field label="提前天数"><input value={days} placeholder="30, 7, 0" maxLength={100} disabled={locked} onChange={e=>{setConfirmed(false);setDays(e.target.value);}}/></Field><p className="form-note">0表示到期当日；启用须填写1至12个不重复的0至365天整数，以逗号分隔。没有自动选用的公司规则。</p><Field label="策略变更原因"><textarea required rows={2} maxLength={200} value={reason} disabled={locked} onChange={e=>{setConfirmed(false);setReason(e.target.value);}}/></Field><label className="checkbox-row"><input type="checkbox" checked={confirmed} disabled={locked} onChange={e=>setConfirmed(e.target.checked)}/>已核对组织范围、时区、提前量及历史提醒处理方式。</label><MFAConfirmation onFresh={setFresh}/><ErrorNotice error={publish.error}/>{blocked&&<p className="notice neutral">本次结果或权限需重新核对，请刷新提醒策略并检查发布历史，再决定下一步。</p>}<div className="form-actions"><button className="button primary" disabled={!confirmed||!fresh||!reason.trim()||!timezone||locked}>{publish.isPending?'正在发布…':'确认发布提醒策略'}</button><button type="button" className="button secondary" disabled={publish.isPending} onClick={cancel}>关闭策略表单</button></div></form>;
}
