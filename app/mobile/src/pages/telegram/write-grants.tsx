import { useState } from 'react';
import Taro from '@tarojs/taro';
import { Text,View } from '@tarojs/components';
import { grantCancellationAccepted,grantStateLabels,grantToolLabels,untriggeredGrant } from '@copypaste/contracts/telegram-grants';
import { useCollection } from '../../platform/collection';
import { useCommand,CommandNotice } from '../../platform/command';
import type { Schemas } from '../../platform/session';
import { Action,Busy,Check,Empty,Notice,Page,Section,date } from '../../ui';
import { SelectField,TextField } from '../business/controls';

export default function TelegramWriteGrantsPage(){
 const [state,setState]=useState('');
 return <Page title="我的固定预授权" home><View className="cp-pad"><Text>查询不会触发执行。每份授权只允许一个已核对的准确项目变更；实际结果与群内披露分别核对。</Text><SelectField label="预授权状态" disabled={false} value={state} options={[[ '', '全部状态' ],...Object.entries(grantStateLabels).map(([k,v])=>[k,v] as const)]} onChange={setState}/></View><GrantRows key={state} state={state}/><Action secondary onClick={()=>void Taro.redirectTo({url:'/pages/telegram/index'})}>返回本人TG入口</Action></Page>;
}
function GrantRows({state}:{state:string}){
 const q=useCollection<Schemas['TelegramWriteGrant']>('/me/telegram/write-grants?limit=20'+(state?'&state='+encodeURIComponent(state):''));
 return <Section title="当前有权查阅的本人预授权"><View className="cp-pad"><Text>原登录、权限、群或业务依据变化后须重新建立授权；取消不会撤销已完成业务。</Text><Notice error={q.error}/>{q.loading?<Busy/>:!q.error&&<>{q.items.map(v=><View className="cp-pad" key={v.id+':'+v.run_version+':'+v.state}><Text className="cp-row-title">{grantToolLabels[v.tool]}</Text><Text className="cp-paragraph">{grantStateLabels[v.state]} · 执行状态 {v.run_state} · v{v.run_version}</Text><Text className="cp-paragraph">建立于 {date(v.created_at)} · 触发到期 {date(v.expires_at)}</Text>{v.state==='pending_trigger'&&<Text className="cp-grant-trigger" selectable>{v.trigger}</Text>}<Action secondary onClick={()=>void Taro.navigateTo({url:'/pages/secretary/index?kind=conversation&id='+v.conversation_id+'&run='+v.run_id})}>查看准确指令与实际结果</Action>{untriggeredGrant(v)&&<CancelGrant value={v} saved={q.reload}/>}</View>)}{q.items.length===0&&<Empty>{q.next?'本页无当前可见项目，请继续读取剩余清单。':'当前清单没有符合条件的预授权。'}</Empty>}</>}{q.next&&!q.error&&<Action secondary loading={q.moreBusy} onClick={()=>void q.more()}>继续读取更早预授权</Action>}<Action secondary loading={q.loading} onClick={q.reload}>刷新预授权清单</Action></View></Section>;
}
function CancelGrant({value,saved}:{value:Schemas['TelegramWriteGrant'];saved():void}){
 const[reason,setReason]=useState(''),[reviewed,setReviewed]=useState(false);
 const command=useCommand<Schemas['Run']>(saved,'tg-grant-cancel:'+value.id+':'+value.run_version,v=>grantCancellationAccepted(v,value));
 return <View><TextField name={'grant-cancel-'+value.id} label="取消这份预授权的原因" value={reason} max={200} disabled={command.busy||command.blocked} onChange={v=>{setReason(v);setReviewed(false);}}/><Check checked={reviewed} onChange={setReviewed} disabled={command.busy||command.blocked}>已核对本份授权，申请停止尚未完成的执行。</Check><Notice error={command.error}/><CommandNotice blocked={command.blocked}/><Action secondary loading={command.busy} disabled={command.blocked||!reviewed||!reason.trim()} onClick={()=>void command.submit('/runs/'+value.run_id+'/cancel',{expected_version:value.run_version,action:'cancel',reason})}>取消此预授权执行</Action></View>;
}
