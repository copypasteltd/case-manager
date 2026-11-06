import { useState } from 'react';
import { Text, View } from '@tarojs/components';
import { checkedTerminalCandidate, checkedTerminalObservation, objectOutcomes, terminalBlockers, terminalNotice, terminalObservationInput, terminalOutcomes, type TerminalCandidate, type TerminalObservation, type TerminalOverview } from '@copypaste/contracts/terminal-observations';
import { useCollection, useRecord } from '../../platform/collection';
import { CommandNotice, MFAConfirmation, useCommand } from '../../platform/command';
import type { Schemas } from '../../platform/session';
import { Action, Busy, Empty, Fact, Notice, Section, date } from '../../ui';
import { SelectField, TextField } from '../business/controls';
export function TerminalObservations({folder,me}:{folder:Schemas['Folder'];me:Schemas['Me']}) {
 const [open,setOpen]=useState(false),[id,setID]=useState('');
 return <Section title='终止上传的准确身份核对'><Text className='cp-paragraph'>{terminalNotice}</Text><Action secondary onClick={()=>setOpen(v=>!v)}>{open?'收起准确身份盘点':'盘点已过期的已知上传'}</Action>{open&&<TerminalList key={folder.id+':'+folder.version+':'+me.authorization_version} folder={folder} onSelect={setID}/ >}{id&&<TerminalWorkspace key={id+':'+me.authorization_version} id={id} me={me} onClose={()=>setID('')}/>}</Section>;
}
function TerminalList({folder,onSelect}:{folder:Schemas['Folder'];onSelect(id:string):void}) {
 const q=useCollection<TerminalCandidate>('/folders/'+folder.id+'/terminal-upload-candidates?limit=20');
 return <View className='cp-pad'><Notice error={q.error}/>{q.loading||q.moreBusy?<Busy/>:!q.error&&<>{q.items.map(v=>{checkedTerminalCandidate(v,folder.id);return <View key={v.upload_id} className='cp-card'><Text className='cp-heading'>{v.name}</Text><Fact label='准确已知位置'>{v.object_count}项 · 上传版本{v.upload_version}</Fact><Action secondary onClick={()=>onSelect(v.upload_id)}>核对来源与观察回执</Action></View>})}{!q.items.length&&<Empty>没有可见的已过期上传。</Empty>}</>}{q.next&&<Action secondary disabled={q.moreBusy} onClick={()=>void q.more()}>继续盘点</Action>}<Action secondary onClick={q.reload}>重新盘点当前上传</Action></View>;
}
function TerminalWorkspace({id,me,onClose}:{id:string;me:Schemas['Me'];onClose():void}) {
 const q=useRecord<TerminalOverview>('/uploads/'+id+'/terminal-observations');const [reason,setReason]=useState(''),[reviewed,setReviewed]=useState(false),[fresh,setFresh]=useState(false);
 const command=useCommand<TerminalObservation>(()=>{setReviewed(false);q.reload();},JSON.stringify(['terminal-observe',id,q.value?.candidate.basis_sha256,me.user.id,me.authorization_version]),v=>{checkedTerminalObservation(v,id,q.value?.candidate.basis_sha256);return true;});
 const disabled=command.busy||command.blocked;const v=q.loading||q.error?undefined:q.value;
 function submit(){if(v)void command.submit('/uploads/'+id+'/terminal-observations',terminalObservationInput(v.candidate,reason,reviewed));}
 return <Section title='准确原生身份观察'><View className='cp-pad'><Notice error={q.error||command.error}/><CommandNotice blocked={command.blocked}/>{q.loading?<Busy/>:v&&<><Text className='cp-heading'>{v.candidate.name}</Text><Fact label='固定已知位置'>{v.candidate.object_count}项 · 上传版本{v.candidate.upload_version}</Fact>{v.candidate.blocking_reasons.length>0&&<Text className='cp-notice'>保护条件尚未满足：{v.candidate.blocking_reasons.map(code=>terminalBlockers[code]??'请重新核对当前保护').join('、')}</Text>}{v.candidate.can_observe&&<><TextField name='terminal-reason' label='观察原因' value={reason} disabled={disabled} onChange={setReason}/><SelectField label='本人核对全部已知位置，只授权读取元数据' value={reviewed?'yes':'no'} options={[["no","尚未确认"],["yes","已核对，确认只读观察"]]} onChange={v=>setReviewed(v==='yes')} disabled={disabled}/><MFAConfirmation disabled={disabled} onFresh={setFresh}/><Action disabled={disabled||!fresh||!reviewed||!reason.trim()} loading={command.busy} onClick={submit}>申请准确原生观察</Action></>}{v.items.map(r=>{checkedTerminalObservation(r,id);return <View key={r.id} className='cp-card'><Text className='cp-heading'>{terminalOutcomes[r.outcome]}</Text><Fact label='观察原因'>{r.reason}</Fact><Fact label='原授权至'>{date(r.expires_at)}</Fact><Text>{r.all_present?'所有已知位置均有准确来源，仍须另行清理核对':'来源未完整核实，不解除清理保护'}</Text>{r.objects.map(o=><Fact key={o.target_id+o.role} label={o.provider.toUpperCase()+' · '+(o.role==='upload_draft'?'上传草稿':'提升候选')}>{objectOutcomes[o.outcome]}</Fact>)}</View>})}{v.has_more&&<Text>当前显示最近50份不可变观察记录。</Text>}</>}<Action secondary disabled={command.busy} onClick={q.reload}>重新读取当前来源与回执</Action><Action secondary disabled={command.busy} onClick={onClose}>关闭准确观察</Action></View></Section>;
}
