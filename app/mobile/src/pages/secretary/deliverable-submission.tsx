import { Text,View } from '@tarojs/components';
import { useState } from 'react';
import { makeDeliverableSubmission } from '@copypaste/contracts/deliverable-submission';
import type { Mutation } from '@copypaste/contracts/mutation';
import type { Schemas } from '../../platform/session';
import { FixedFilePicker, type FixedFile } from '../../platform/fixed-files';
import { Action, Notice, Check } from '../../ui';
import { TextField } from '../business/controls';
import { MobileError } from '../../platform/session';

export function ConversationDeliverableSubmission({target,disabled,onChange}:{target:Schemas['Deliverable'];disabled:boolean;onChange(value:Mutation):void}) {
  const [files,setFiles]=useState<FixedFile[]>([]),[description,setDescription]=useState(''),[reason,setReason]=useState(''),[reviewed,setReviewed]=useState(false),[error,setError]=useState<MobileError>();
  return <View><Notice error={error}/><Text className='cp-small cp-paragraph'>核对固定文件版本与原修订；秘书只提交待验收，独立人员再决定验收。</Text>{target.state==='accepted'&&<Text className='cp-small cp-paragraph'>本次创建新的待验收修订，保留此前验收历史。</Text>}
    <TextField name='submission-description' label='交付说明' multiline max={20000} value={description} disabled={disabled} onChange={v=>{setDescription(v);setReviewed(false);}}/>
    <FixedFilePicker projectID={target.project_id} value={files} onChange={v=>{setFiles(v);setReviewed(false);}} disabled={disabled} purpose='秘书本次交付'/>
    <TextField name='submission-reason' label='本次提交原因' value={reason} max={200} disabled={disabled} onChange={v=>{setReason(v);setReviewed(false);}}/>
    <Check checked={reviewed} onChange={setReviewed} disabled={disabled}>已核对固定文件和原修订，允许秘书提交待验收</Check>
    <Action secondary disabled={disabled||!target.can_submit||!reviewed} onClick={()=>{try{onChange(makeDeliverableSubmission(target,files.map(f=>f.version_id),description,reason,reviewed));}catch(cause){setError(new MobileError('INPUT_REQUIRED',cause instanceof Error?cause.message:'请核对本次交付。'));}}}>将提交验收加入请求</Action>
  </View>;
}
