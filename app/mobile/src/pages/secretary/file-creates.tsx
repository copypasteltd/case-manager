import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { makeFileCreate, type FileCreate } from '@copypaste/contracts/file-update';
import { useRecord } from '../../platform/collection';
import { type Schemas, MobileError } from '../../platform/session';
import { Action, Busy, Notice } from '../../ui';
import { ChoiceList, SelectField, TextField } from '../business/controls';

export function ConversationFileCreate({conversation:c,disabled,value,onChange}:{conversation:Schemas['Conversation'];disabled:boolean;value?:FileCreate;onChange(value?:FileCreate):void}) {
  const [open,setOpen]=useState(false), [picking,setPicking]=useState(false), [folderID,setFolderID]=useState(''), [name,setName]=useState(''), [media,setMedia]=useState<'text/plain'|'text/markdown'>('text/markdown'), [error,setError]=useState<unknown>();
  const current=useRecord<Schemas['Folder']>(open&&folderID?'/folders/'+folderID:null);
  const folder=current.error?undefined:current.value;
  if(picking)return <ChoiceList<Schemas['Folder']> title={folder?'选择子目录：'+folder.name:'选择新文件资料目录'} path={'/folders?limit=20'+(c.project_id?'&project_id='+c.project_id:'')+(folderID?'&parent_id='+folderID:'')} searchable={false} label={f=>f.name} eligible={f=>f.state==='active'&&f.project_id===c.project_id} onSelect={f=>{if(!disabled){setFolderID(f.id);setPicking(false);setError(undefined);}}} onClose={()=>setPicking(false)}/>;
  return <View><Text className='cp-row-title'>本次准确文件新建</Text>{value?<><Text className='cp-paragraph'>{value.name} · 目录修订 {value.expected_folder_version}</Text><Text className='cp-small cp-paragraph'>只在所选目录新建这一文件，重名会停止；请在消息中说明正文要求。</Text><Action secondary disabled={disabled} onClick={()=>{onChange(undefined);setOpen(false);}}>移除本次文件新建</Action></>:!open?<Action secondary disabled={disabled} onClick={()=>{setOpen(true);setPicking(true);}}>选择新建文件位置</Action>:<>
    <Notice error={error??current.error}/>{current.loading&&<Busy/>}
    {folder?<><Text className='cp-paragraph'>目标：{folder.name} · 修订 {folder.version}</Text><Action secondary disabled={disabled} onClick={()=>setPicking(true)}>浏览子目录</Action>{folder.parent_id&&<Action secondary disabled={disabled} onClick={()=>setFolderID(folder.parent_id!)}>返回上级目录</Action>}</>:<Action secondary disabled={disabled} onClick={()=>{setFolderID('');setPicking(true);}}>重新选择资料目录</Action>}
    <Action secondary disabled={disabled} onClick={()=>{setFolderID('');setPicking(true);}}>选择其他根目录</Action>
    <SelectField label='新文件格式' value={media} options={[[ 'text/markdown','Markdown（.md／.markdown）' ],[ 'text/plain','文本（.txt）' ]]} disabled={disabled} onChange={v=>setMedia(v as typeof media)}/>
    <TextField name='file-create-name' label='新文件名称' value={name} onChange={setName} disabled={disabled}/>
    <Text className='cp-small cp-paragraph'>选择后仍需发送正文要求；新建正文最多48KiB，经校验与扫描后才成为正式文件。</Text>
    <Action secondary disabled={disabled||!folder||current.loading||!name.trim()} onClick={()=>{try{onChange(makeFileCreate(folder!,name,media,c.project_id));setOpen(false);}catch(cause){setError(new MobileError('INPUT_REQUIRED',cause instanceof Error?cause.message:'请核对新建文件位置。'));}}}>将准确文件新建加入请求</Action>
    <Action secondary disabled={disabled} onClick={()=>setOpen(false)}>收起文件新建</Action>
  </>}</View>;
}
