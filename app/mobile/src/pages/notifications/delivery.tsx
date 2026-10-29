import { Text, View } from '@tarojs/components';
import { useEffect, useRef, useState } from 'react';
import { checkedDeliveryJob, checkedRetryReceipt, notificationRetryBlocks, notificationRetryInput, type DeliveryJob, type RetryReceipt } from '@copypaste/contracts/notifications';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import type { Schemas } from '../../platform/session';
import { Action, Busy, Empty, Fact, Notice, Section, State, date } from '../../ui';
import { TextField } from '../business/controls';
import { Checkbox } from '../../platform/components';

export function NotificationDelivery({me}:{me:Schemas['Me']}) {
  const failures=useCollection<Schemas['NotificationFailure']>('/notification-delivery-failures?limit=10');
  const history=useCollection<RetryReceipt>('/notification-delivery-retries?limit=10');
  const [selected,setSelected]=useState<string>(),[revision,setRevision]=useState(0);
  function refresh(){failures.reload();history.reload();setRevision(v=>v+1);}
  function open(id:string){setSelected(id);setRevision(v=>v+1);}
  let receipts:RetryReceipt[]=[];let historyError=history.error;
  try {receipts=history.items.map(r=>checkedRetryReceipt(r));}catch(cause){historyError=cause;}
  return <>
    <Section title='投递异常与重试'><View className='cp-pad'><Text className='cp-muted cp-paragraph'>查看组织内的任务技术状态。人工重试保留原失败，并重新核对接收人当前权限。</Text><Action secondary disabled={failures.loading||history.loading} onClick={refresh}>刷新投递记录</Action><Notice error={failures.error}/>{failures.loading?<Busy/>:!failures.error&&(failures.items.length?failures.items.map(item=><View className='cp-paragraph' key={item.job_id}><Fact label={item.safe_code??'等待核对'}><State value={item.state}/><Text>尝试 {item.attempt} / {item.max_attempts}</Text><Text className='cp-muted cp-small'>{date(item.updated_at)}</Text><Text className='cp-code' selectable>{item.job_id}</Text></Fact><Action secondary onClick={()=>open(item.job_id)}>核对任务与重试</Action></View>):<Empty>当前没有投递异常。</Empty>)}{failures.next&&<Action secondary loading={failures.moreBusy} onClick={()=>void failures.more()}>加载更早异常</Action>}</View></Section>
    {selected&&<DeliveryDetail key={selected+':'+revision} id={selected} me={me} open={open} refresh={refresh}/>}
    <Section title='重试历史'><View className='cp-pad'><Notice error={historyError}/>{history.loading?<Busy/>:!historyError&&(receipts.length?receipts.map(item=><View className='cp-paragraph' key={item.id}><Text>{date(item.created_at)}</Text><Fact label='原失败任务'><Text className='cp-code' selectable>{item.source_job_id}</Text></Fact><Fact label='新任务'><Text className='cp-code' selectable>{item.retry_job_id}</Text></Fact><Action secondary onClick={()=>open(item.source_job_id)}>查看原失败</Action><Action onClick={()=>open(item.retry_job_id)}>查看重试结果</Action></View>):<Empty>尚无人工重试记录。</Empty>)}{history.next&&!historyError&&<Action secondary loading={history.moreBusy} onClick={()=>void history.more()}>加载更早重试</Action>}</View></Section>
  </>;
}
function DeliveryDetail({id,me,open,refresh}:{id:string;me:Schemas['Me'];open(id:string):void;refresh():void}) {
  const record=useRecord<DeliveryJob>('/notification-delivery-jobs/'+id);
  let job:DeliveryJob|undefined,error=record.error;
  try {if(record.value)job=checkedDeliveryJob(record.value,id);}catch(cause){error=cause;}
  const recheck=useRef(refresh);recheck.current=refresh;
  useEffect(()=>{if(!['queued','running','retry_wait'].includes(job?.state??''))return;const timer=setTimeout(()=>recheck.current(),3000);return()=>clearTimeout(timer);},[job?.job_id,job?.state,job?.updated_at]);
  return <Section title='所选投递任务'><View className='cp-pad'><Notice error={error}/>{record.loading?<Busy/>:!error&&job&&<>
    <Text className='cp-code' selectable>{job.job_id}</Text><Fact label='当前状态'><State value={job.state}/><Text>尝试 {job.attempt} / {job.max_attempts} · {job.safe_code??'尚无失败码'}</Text></Fact><Text className='cp-paragraph'>{notificationRetryBlocks[job.retry_block]}</Text>
    {job.state==='retry_wait'&&<Text className='cp-muted cp-small'>自动重试最早时间：{date(job.available_at)}</Text>}
    {job.business_event_id&&<Fact label='固定业务事件'><Text>{job.event_type}</Text><Text className='cp-code' selectable>{job.business_event_id}</Text></Fact>}
    {job.result&&<Text className='cp-notice'>已写入站内信 {job.result.delivered} 条，略过 {job.result.suppressed} 条；{job.result.safe_code}。此结果不表示员工已读或 TG 已发送。</Text>}
    {job.parent_job_id&&<Action secondary onClick={()=>open(job!.parent_job_id!)}>查看上一步失败</Action>}{job.retry_job_id&&<Action secondary onClick={()=>open(job!.retry_job_id!)}>查看已创建的后续任务</Action>}
    {job.can_retry&&<RetryForm key={job.revision} job={job} me={me} committed={r=>{refresh();open(r.retry_job_id);}}/>}
  </>}</View></Section>;
}
function RetryForm({job,me,committed}:{job:DeliveryJob;me:Schemas['Me'];committed(r:RetryReceipt):void}) {
  const [reason,setReason]=useState(''),[fresh,setFresh]=useState(false),[confirmed,setConfirmed]=useState(false),[error,setError]=useState<unknown>();
  const command=useCommand<RetryReceipt>(committed,[me.user.id,me.authorization_version,'notification-retry',job.job_id,job.revision].join(':'),r=>{checkedRetryReceipt(r,job.job_id,me.user.id,job.business_event_id??undefined);return true;});
  async function submit(){try {setError(undefined);await command.submit('/notification-delivery-jobs/'+job.job_id+'/retry',notificationRetryInput(job,reason));}catch(cause){setError(cause);}}
  return <View><TextField name='notification-retry-reason' label='人工重试原因' value={reason} max={200} disabled={command.busy||command.blocked} onChange={setReason}/><View className='cp-check' onClick={()=>{if(!command.busy&&!command.blocked)setConfirmed(v=>!v);}}><Checkbox value='confirmed' ariaLabel='已核对失败原因' checked={confirmed} disabled={command.busy||command.blocked}/><Text>已核对失败原因；按原事件重新检查当前接收人，不修改原始业务。</Text></View><MFAConfirmation disabled={command.busy||command.blocked} onFresh={setFresh}/><Notice error={error??command.error}/><CommandNotice blocked={command.blocked}/><Action disabled={!fresh||!confirmed||!reason.trim()||command.blocked} loading={command.busy} onClick={()=>void submit()}>确认创建重试</Action></View>;
}
