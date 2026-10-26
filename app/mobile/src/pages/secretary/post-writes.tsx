import { Text, View } from '@tarojs/components';
import { useEffect, useState } from 'react';
import { makePostDraft, makePostPublish, postTypes, postWriteReceipt, postWriteScopeAllowed, type PostWrite } from '@copypaste/contracts/post-write';
import { useRecord } from '../../platform/collection';
import { type Schemas } from '../../platform/session';
import { go } from '../../platform/page';
import { Action, Busy, Check, Notice } from '../../ui';
import { ChoiceList, SelectField, TextField } from '../business/controls';

export function ConversationPostWrite({conversation:c,disabled,value,references,onChange}:{conversation:Schemas['Conversation'];disabled:boolean;value?:PostWrite;references:Schemas['ResourceRef'][];onChange(value?:PostWrite):void}){
 const [open,setOpen]=useState(false),[picking,setPicking]=useState(false),[mode,setMode]=useState<'new'|'revise'|'publish'>('new'),[id,setID]=useState(''),[title,setTitle]=useState(''),[type,setType]=useState('project_log'),[classification,setClassification]=useState(c.project_id?'project_restricted':'internal'),[why,setWhy]=useState(''),[reviewed,setReviewed]=useState(false),[error,setError]=useState<unknown>();
 const current=useRecord<Schemas['Post']>(open&&mode!=='new'&&id?'/posts/'+id:null),p=current.error?undefined:current.value;
 useEffect(()=>{setReviewed(false);setError(undefined);if(p){setTitle(p.title);setType(p.type??'project_log');setClassification(p.classification??'internal');}},[id,p?.version,p?.content_hash]);
 if(c.type!=='private')return <Text className='cp-paragraph'>知识写入请进入本人私密会话。</Text>;
 if(picking)return <ChoiceList<Schemas['Post']> title='选择知识文档' path={'/posts?limit=20'+(c.project_id?'&project_id='+c.project_id:'')} searchable label={p=>p.title} eligible={p=>p.project_id===c.project_id&&p.state!=='archived'} onSelect={p=>{if(!disabled){setID(p.id);setPicking(false);}}} onClose={()=>setPicking(false)}/>;
 const attachments=references.filter(r=>r.type==='file'&&!!r.version_id);
 return <View className='cp-editor-row'><Text className='cp-row-title'>本次知识草稿或内部发布</Text>{value?<><Text className='cp-paragraph'>{value.tool==='publish_post'?'发布已核对的准确修订':value.post_id?'生成并保存指定文档的草稿修订':'生成并保存新的知识草稿'}</Text><Text className='cp-muted cp-paragraph'>已固定本次目标；正文写作要求请填写在发给秘书的请求中。</Text><Action secondary disabled={disabled} onClick={()=>{onChange(undefined);setOpen(false);}}>移除本次知识操作</Action></>:!open?<Action secondary disabled={disabled} onClick={()=>setOpen(true)}>选择知识草稿或发布</Action>:<>
 <Notice error={error??current.error}/><SelectField label='知识操作' value={mode} options={[[ 'new','新建知识草稿' ],[ 'revise','修订知识草稿' ],[ 'publish','核对后内部发布' ]]} disabled={disabled} onChange={v=>{setMode(v as typeof mode);setID('');setReviewed(false);}}/>
 {mode!=='new'&&<Action secondary disabled={disabled} onClick={()=>setPicking(true)}>选择知识文档</Action>}{current.loading&&<Busy/>}
 {mode!=='publish'&&<><TextField name='knowledge-title' label='知识标题' max={200} value={title} disabled={disabled} onChange={setTitle}/><SelectField label='知识类型' value={type} options={Object.entries(postTypes)} disabled={disabled||mode==='revise'} onChange={setType}/><SelectField label='知识分类' value={classification} options={(['internal','project_restricted'] as const).filter(v=>postWriteScopeAllowed(v,c.classification)&&!!(c.project_id||v==='internal')).map(v=>[v,v==='internal'?'内部':'项目受限'])} disabled={disabled||mode==='revise'} onChange={setClassification}/><Text className='cp-muted cp-paragraph'>附件使用下方已选择的准确文件版本，共 {attachments.length} 项；仅保存草稿，原发布版保持有效。</Text></>}
 {p&&<><Text className='cp-paragraph'>文档修订 {p.version} · {p.state} · {p.classification}</Text><Text className='cp-code' selectable>{p.content_hash}</Text><Action secondary onClick={()=>go('post',p.id)}>查看知识详情与历史</Action></>}
 {mode==='publish'&&p&&<><Text className='cp-row-title'>本次完整内部发布内容</Text><Text className='cp-row-title'>{p.title}</Text><Text className='cp-paragraph' selectable>{p.body}</Text><Text className='cp-paragraph'>摘要：{p.summary||'（无）'}</Text><Text className='cp-paragraph'>标签：{p.tags?.join('、')||'（无）'}</Text>{p.source_refs?.map(r=><Text className='cp-paragraph' key={r.id+(r.version_id??'')}>文件 {r.id} · 固定版本 {r.version_id??'未固定，不能发布'}</Text>)}<Check checked={reviewed} disabled={disabled} onChange={setReviewed}>我已核对完整正文、摘要、标签、分类和固定附件，确认内部发布</Check></>}
 <TextField name='knowledge-reason' label='知识操作说明' value={why} max={200} disabled={disabled} onChange={setWhy}/>
 <Action secondary disabled={disabled||!why.trim()||(mode!=='new'&&(!p||current.loading))||(mode==='publish'&&!reviewed)} onClick={()=>{try{onChange(mode==='publish'?makePostPublish(p!,c.project_id,why):makePostDraft(c.project_id,type,title,classification,attachments,why,mode==='revise'?p:undefined));setOpen(false);}catch(e){setError(e);}}}>将知识操作加入本次请求</Action><Action secondary disabled={disabled} onClick={()=>setOpen(false)}>收起知识选择</Action>
 </>}</View>;
}
export function PostWriteReceipt({run}:{run:Schemas['Run']}){
 const v=run.post_write!,p=v.result;
 return <View className='cp-editor-row'><Text className='cp-row-title'>知识执行记录</Text>{postWriteReceipt(run)&&p?<><Text className='cp-paragraph'>{v.input.tool==='publish_post'?'已发布准确内部修订':'知识草稿已保存，发布需另行核对'} · {p.title}</Text><Text className='cp-paragraph'>文档修订 {p.version} · 操作 {v.operation_id}</Text><Action secondary onClick={()=>go('post',p.id)}>查阅完整知识内容</Action></>:<Text className='cp-paragraph'>已记录本次目标，请以真实保存记录核对执行结果。</Text>}</View>;
}
