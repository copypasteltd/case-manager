import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { makeOATextAdopt, oaTextName, oaTextOriginal, oaTextProven } from '@copypaste/contracts/oa-text';
import { oaKindNames } from '@copypaste/contracts/oa-request';
import { CommandNotice, useCommand } from '../../platform/command';
import { go } from '../../platform/page';
import { type Schemas } from '../../platform/session';
import { Action, Check, Notice } from '../../ui';
import { TextField } from '../business/controls';

export function OATextReview({run}:{run:Schemas['Run']}) {
 const base=run.oa_request!;
 const [saved,setSaved]=useState<Schemas['AgentOARequestView']>(),[reviewed,setReviewed]=useState(false),[reason,setReason]=useState(''),[error,setError]=useState<unknown>();
 const v=saved??base,d=v.text_draft,proven=oaTextProven(v),adopted=proven&&v.status==='adopted';
 const command=useCommand<Schemas['AgentOARequestView']>(value=>{setSaved(value);setReason('');setReviewed(false);},'oa-text-adopt:'+run.id+':'+(d?.draft_sha256??''),value=>value.run_id===run.id&&value.intent_sha256===base.intent_sha256&&value.text_draft?.draft_sha256===d?.draft_sha256&&value.status==='adopted'&&oaTextProven(value));
 return <View className='cp-editor-row'><Text className='cp-row-title'>业务正文草稿</Text><Text className='cp-paragraph'>{oaKindNames[v.kind]} · {v.before.business.title}</Text><Notice error={error??command.error}/><CommandNotice blocked={command.blocked}/>
 {!proven?<Text className='cp-paragraph'>已记录正文起草目标，等待秘书保存完整建议。</Text>:<>
 <Text className='cp-muted cp-paragraph'>{adopted?'正文已采用并保存为业务新修订。':'逐字段核对原文与建议，确认后保存业务新修订。'}</Text>
 {Object.entries(d!.fields).map(([field,text])=><View className='cp-editor-row' key={field}><Text className='cp-row-title'>{oaTextName(field,v.before.business.payload.content)}</Text><Text className='cp-muted cp-paragraph'>原文</Text><Text selectable className='cp-paragraph'>{oaTextOriginal(v,field)||'（空白）'}</Text><Text className='cp-muted cp-paragraph'>正文建议</Text><Text selectable className='cp-paragraph'>{text}</Text></View>)}
 {adopted?<Text className='cp-code' selectable>业务修订 {d!.result!.version} · 操作 {d!.operation_id}</Text>:run.state==='succeeded'&&run.outcome==='draft'&&!run.cancel_requested_at?<><Check checked={reviewed} disabled={command.busy||command.blocked} onChange={setReviewed}>我已核对全部正文建议，确认采用</Check><TextField name='oa-text-adopt-reason' label='采用正文的依据' value={reason} max={200} disabled={command.busy||command.blocked} onChange={setReason}/><Action secondary disabled={command.busy||command.blocked||!reviewed||!reason.trim()} onClick={()=>{try{void command.submit('/runs/'+run.id+'/oa-text-adoption',makeOATextAdopt(v,reviewed,reason));}catch(e){setError(e);}}}>采用正文草稿</Action></>:<Text className='cp-paragraph'>请等待本次任务结束；失败或取消的任务不能采用正文。</Text>}
 <Text className='cp-muted cp-paragraph'>保存后可另行准备申请和核对送审。</Text></>}
 <Action secondary onClick={()=>go(v.kind,v.business_id)}>查看完整业务资料</Action></View>;
}
