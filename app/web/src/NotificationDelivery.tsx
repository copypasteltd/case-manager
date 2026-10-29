import { useRef, useState } from 'react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { checkedDeliveryJob, checkedRetryReceipt, notificationRetryBlocks, notificationRetryInput, type DeliveryJob, type RetryReceipt } from '@copypaste/contracts/notifications';
import { api, ApiError, type Me, type Page, type Schemas } from './api';
import { Busy, date, Empty, ErrorNotice, Field, State } from './components';
import { MFAConfirmation } from './MFAConfirmation';

export default function NotificationDelivery({ me }: { me: Me }) {
  const [selected,setSelected]=useState<string>(), [revision,setRevision]=useState(0);
  const cache=useQueryClient(), authority=[me.user.id,me.authorization_version];
  const failures=useInfiniteQuery({queryKey:['notification-failures',...authority],initialPageParam:'',queryFn:({pageParam,signal})=>api<Page<Schemas['NotificationFailure']>>('/notification-delivery-failures?limit=10'+(pageParam?'&cursor='+encodeURIComponent(pageParam):''),{signal}),getNextPageParam:p=>p.next_cursor??undefined,refetchInterval:30000,gcTime:0});
  const history=useInfiniteQuery({queryKey:['notification-retries',...authority],initialPageParam:'',queryFn:async({pageParam,signal})=>{const p=await api<Page<RetryReceipt>>('/notification-delivery-retries?limit=10'+(pageParam?'&cursor='+encodeURIComponent(pageParam):''),{signal});p.items.forEach(r=>checkedRetryReceipt(r));return p;},getNextPageParam:p=>p.next_cursor??undefined,refetchInterval:30000,gcTime:0});
  const detail=useQuery({queryKey:['notification-delivery-job',...authority,selected,revision],queryFn:async({signal})=>checkedDeliveryJob(await api<DeliveryJob>('/notification-delivery-jobs/'+selected,{signal}),selected!),enabled:!!selected,refetchInterval:q=>['queued','running','retry_wait'].includes(q.state.data?.state??'')?3000:false,gcTime:0});
  async function refresh(){setRevision(v=>v+1);await Promise.all([cache.invalidateQueries({queryKey:['notification-failures']}),cache.invalidateQueries({queryKey:['notification-retries']})]);}
  function open(id:string){setSelected(id);setRevision(v=>v+1);}
  return <section className="panel conversation-summary" aria-label="投递异常与重试"><div className="section-heading"><h2>投递异常与重试</h2><button className="button secondary" disabled={failures.isFetching||history.isFetching||detail.isFetching} onClick={()=>void refresh()}>刷新投递记录</button></div>
    <p className="form-note">查看组织内的任务技术状态。人工重试会保留原失败记录，并重新核对接收人当前权限；业务资料和原始事件保持固定。</p>
    <ErrorNotice error={failures.error??history.error??detail.error}/>
    {failures.isPending?<Busy/>:!failures.error&&<><h3>当前异常</h3>{failures.data?.pages.flatMap(p=>p.items).map(item=><article className="conversation-source-preview" key={item.job_id}><State state={item.state}/><p>{item.safe_code??'等待核对'} · 尝试 {item.attempt} / {item.max_attempts} · {date(item.updated_at)}</p><code>{item.job_id}</code><div className="form-actions"><button className="button secondary" onClick={()=>open(item.job_id)}>核对任务与重试</button></div></article>)}{!failures.data?.pages.some(p=>p.items.length)&&<Empty title="当前没有投递异常" detail="已提交的重试仍可在下方历史中查看。"/>}{failures.hasNextPage&&<button className="button secondary" disabled={failures.isFetchingNextPage} onClick={()=>void failures.fetchNextPage()}>加载更早异常</button>}</>}
    {selected&&(detail.isPending?<Busy/>:!detail.error&&detail.data&&<DeliveryDetail key={selected+':'+detail.data.revision+':'+revision} job={detail.data} me={me} refresh={refresh} open={open}/>)}
    <h3>重试历史</h3>{history.isPending?<Busy/>:!history.error&&<>{history.data?.pages.flatMap(p=>p.items).map(item=><article className="conversation-source-preview" key={item.id}><p>{date(item.created_at)} · 经办人 {item.created_by}</p><p>原失败任务 <code>{item.source_job_id}</code></p><p>新任务 <code>{item.retry_job_id}</code></p><div className="form-actions"><button className="button secondary" onClick={()=>open(item.source_job_id)}>查看原失败</button><button className="button" onClick={()=>open(item.retry_job_id)}>查看重试结果</button></div></article>)}{!history.data?.pages.some(p=>p.items.length)&&<p className="muted">尚无人工重试记录。</p>}{history.hasNextPage&&<button className="button secondary" disabled={history.isFetchingNextPage} onClick={()=>void history.fetchNextPage()}>加载更早重试</button>}</>}
  </section>;
}

function DeliveryDetail({job,me,refresh,open}:{job:DeliveryJob;me:Me;refresh():Promise<void>;open(id:string):void}) {
  const [reason,setReason]=useState(''),[fresh,setFresh]=useState(false),[confirmed,setConfirmed]=useState(false),[blocked,setBlocked]=useState(false);
  const pending=useRef<{body:string;key:string}>();
  const retry=useMutation({mutationFn:async()=>{
    const body=notificationRetryInput(job,reason),encoded=JSON.stringify(body);
    if(!pending.current||pending.current.body!==encoded)pending.current={body:encoded,key:crypto.randomUUID()};
    let returned=false;
    try { const r=await api<RetryReceipt>('/notification-delivery-jobs/'+job.job_id+'/retry',{method:'POST',body,key:pending.current.key});returned=true;return checkedRetryReceipt(r,job.job_id,me.user.id,job.business_event_id??undefined); }
    catch(cause){if(returned||!(cause instanceof ApiError)||cause.status===0||cause.status>=500||[403,404,409,412].includes(cause.status))setBlocked(true);throw cause;}
  },onSuccess:async r=>{await refresh();open(r.retry_job_id);}});
  return <article className="conversation-source-preview" aria-label="所选投递任务"><h3>所选投递任务</h3><code>{job.job_id}</code><p><State state={job.state}/> · 尝试 {job.attempt} / {job.max_attempts} · {job.safe_code??'尚无失败码'}</p><p>{notificationRetryBlocks[job.retry_block]}</p>{job.state==='retry_wait'&&<p>自动重试最早时间：{date(job.available_at)}</p>}
    {job.business_event_id&&<p className="muted">业务事件 {job.event_type} · <code>{job.business_event_id}</code></p>}
    {job.result&&<p className="notice neutral">已写入站内信 {job.result.delivered} 条，略过 {job.result.suppressed} 条；{job.result.safe_code}。此结果不表示员工已读或 TG 已发送。</p>}
    {job.parent_job_id&&<button className="button secondary" onClick={()=>open(job.parent_job_id!)}>查看上一步失败</button>}{job.retry_job_id&&<button className="button secondary" onClick={()=>open(job.retry_job_id!)}>查看已创建的后续任务</button>}
    {job.can_retry&&<form onSubmit={e=>{e.preventDefault();if(fresh&&confirmed&&!blocked&&!retry.isPending)retry.mutate();}}><Field label="人工重试原因"><textarea required rows={3} maxLength={200} value={reason} disabled={retry.isPending||blocked} onChange={e=>setReason(e.target.value)}/></Field><label className="checkbox-row"><input type="checkbox" checked={confirmed} disabled={retry.isPending||blocked} onChange={e=>setConfirmed(e.target.checked)}/>已核对失败原因；按原事件重新检查当前接收人，不修改原始业务。</label><MFAConfirmation onFresh={setFresh}/><ErrorNotice error={retry.error}/>{blocked&&<p className="notice neutral">本次结果或权限需要重新核对，请刷新投递记录；保留原请求编号，避免重复创建。</p>}<button className="button primary" disabled={!fresh||!confirmed||!reason.trim()||blocked||retry.isPending}>{retry.isPending?'正在提交…':'确认创建重试'}</button></form>}
  </article>;
}
