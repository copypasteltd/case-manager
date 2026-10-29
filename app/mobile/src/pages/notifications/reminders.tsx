import { Text, View } from '@tarojs/components';
import { useRef, useState } from 'react';
import { checkedContractReminderPolicy, checkedContractReminderReceipt, checkedContractReminderState, contractReminderInput, type ContractReminderInput, type ContractReminderPolicy, type ContractReminderState } from '@copypaste/contracts/notifications';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import { MobileError, type Schemas } from '../../platform/session';
import { Checkbox } from '../../platform/components';
import { Action, Busy, Fact, Notice, Section, date } from '../../ui';
import { SelectField, TextField } from '../business/controls';

export function ContractReminders({me}:{me:Schemas['Me']}) {
  const [epoch,setEpoch]=useState(0);
  return <PolicyControl key={epoch} me={me} refresh={()=>setEpoch(v=>v+1)}/>;
}
function PolicyControl({me,refresh}:{me:Schemas['Me'];refresh():void}) {
  const record=useRecord<ContractReminderState>('/notification-contract-policy');
  const history=useCollection<ContractReminderPolicy>('/notification-contract-policy-history?limit=10');
  const [editing,setEditing]=useState(false);
  let state:ContractReminderState|undefined,error=record.error,entries:ContractReminderPolicy[]=[],historyError=history.error;
  try{if(record.value)state=checkedContractReminderState(record.value);}catch(cause){error=cause;}
  try{entries=history.items.map(checkedContractReminderPolicy);}catch(cause){historyError=cause;}
  return <><Section title='合同到期提醒策略'><View className='cp-pad'><Text className='cp-muted cp-paragraph'>按业务时区与提前量通知当前有权责任人。新策略不补发早于发布业务日的提醒，已入队事件继续按原事实处理；停用不撤回既有通知。</Text><Action secondary disabled={record.loading||history.loading} onClick={refresh}>刷新提醒策略</Action><Notice error={error}/>{record.loading?<Busy/>:!error&&state&&<>{state.current?<PolicyView value={state.current}/>:<Text className='cp-paragraph'>尚未发布组织策略。若 Worker 已明确配置部署级规则，该规则仍可能生效；发布后以组织策略为准。</Text>}{state.can_publish&&!editing&&<Action onClick={()=>setEditing(true)}>修改提醒策略</Action>}{editing&&state.can_publish&&<PolicyForm state={state} me={me} done={refresh} cancel={()=>setEditing(false)}/>}</>}</View></Section><Section title='策略发布历史'><View className='cp-pad'><Notice error={historyError}/>{history.loading?<Busy/>:!historyError&&<>{entries.map(p=><View className='cp-paragraph' key={p.id}><PolicyView value={p}/><Text className='cp-muted'>{p.reason}</Text><Text className='cp-code cp-small'>经办人 {p.created_by}</Text></View>)}{!entries.length&&<Text>暂无已发布策略。</Text>}{history.next&&<Action secondary loading={history.moreBusy} onClick={()=>void history.more()}>加载更早策略</Action>}</>}</View></Section></>;
}
function PolicyView({value:p}:{value:ContractReminderPolicy}){return <Fact label={'版本 '+p.version+' · '+(p.enabled?'已启用':'已停用')}><Text>{p.timezone} · 提前 {p.days_before.length?p.days_before.join('、'):'无'} 天</Text><Text className='cp-muted cp-small'>发布业务日 {p.effective_on} · {date(p.created_at)}</Text></Fact>;}
function PolicyForm({state,me,done,cancel}:{state:ContractReminderState;me:Schemas['Me'];done():void;cancel():void}) {
  const p=state.current;
  const [enabled,setEnabled]=useState(p?.enabled??false),[timezone,setTimezone]=useState(p?.timezone??''),[days,setDays]=useState(p?.days_before.join(', ')??''),[reason,setReason]=useState(''),[confirmed,setConfirmed]=useState(false),[fresh,setFresh]=useState(false),[error,setError]=useState<unknown>();
  const input=useRef<ContractReminderInput>();
  const command=useCommand<ContractReminderPolicy>(done,[me.user.id,me.authorization_version,'contract-reminder-policy',p?.version??0].join(':'),value=>{if(!input.current)throw new Error('本次策略输入尚未固定。');checkedContractReminderReceipt(value,input.current,me.user.id);return true;});
  const locked=command.busy||command.blocked;
  async function submit(){
    setError(undefined);
    try{input.current=contractReminderInput(state,enabled,timezone,days,reason);}
    catch(cause){setError(new MobileError('INVALID_POLICY_INPUT',cause instanceof Error?cause.message:'请核对提醒策略。'));return;}
    await command.submit('/notification-contract-policy',input.current);
  }
  return <View><Text className='cp-paragraph'>基于版本 {p?.version??0}，发布新的不可变版本。</Text><SelectField label='提醒开关' value={enabled?'enabled':'disabled'} options={[[ 'disabled','停用' ],[ 'enabled','启用' ]]} onChange={v=>{setConfirmed(false);setEnabled(v==='enabled');}} disabled={locked}/><TextField name='reminder-timezone' label='业务时区' value={timezone} max={200} disabled={locked} onChange={v=>{setConfirmed(false);setTimezone(v);}} hint='例如 Asia/Hong_Kong，须明确填写。'/><TextField name='reminder-days' label='提前天数' value={days} max={100} disabled={locked} onChange={v=>{setConfirmed(false);setDays(v);}} hint='0表示到期当日；1至12个不重复的0至365天整数，以逗号分隔。'/><TextField name='reminder-reason' label='策略变更原因' value={reason} max={200} multiline disabled={locked} onChange={v=>{setConfirmed(false);setReason(v);}}/><View className='cp-check' onClick={()=>{if(!locked)setConfirmed(v=>!v);}}><Checkbox value='confirmed' ariaLabel='已核对提醒策略' checked={confirmed} disabled={locked}/><Text>已核对组织范围、时区、提前量及历史提醒处理方式。</Text></View><MFAConfirmation disabled={locked} onFresh={setFresh}/><Notice error={error??command.error}/><CommandNotice blocked={command.blocked}/><Action disabled={!confirmed||!fresh||!reason.trim()||!timezone||locked} loading={command.busy} onClick={()=>void submit()}>确认发布提醒策略</Action><Action secondary disabled={command.busy} onClick={cancel}>关闭策略表单</Action></View>;
}
