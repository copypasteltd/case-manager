import { useState } from 'react';
import { makeDeliverableSubmission } from '@copypaste/contracts/deliverable-submission';
import type { Mutation } from '@copypaste/contracts/mutation';
import type { Schemas } from './api';
import { FileVersionPicker, type FileReference } from './BusinessSelectors';
import { ErrorNotice, Field } from './components';

export function ConversationDeliverableSubmission({target,classification,disabled,onChange}:{target:Schemas['Deliverable'];classification:string;disabled:boolean;onChange(value:Mutation):void}) {
  const [files,setFiles]=useState<FileReference[]>([]),[description,setDescription]=useState(''),[reason,setReason]=useState(''),[reviewed,setReviewed]=useState(false),[picking,setPicking]=useState(false),[error,setError]=useState<unknown>();
  const reset=()=>setReviewed(false);
  return <div className="telegram-form"><ErrorNotice error={error}/><p className="form-note">固定本次正式文件版本、说明与原交付修订。秘书只提交新的待验收版本，验收仍由独立人员办理。</p>{target.state==='accepted'&&<p className="notice warning">此交付已有验收结果。本次将创建新的待验收修订，保留旧验收历史。</p>}
    <Field label="交付说明"><textarea rows={4} maxLength={20000} value={description} disabled={disabled} onChange={e=>{setDescription(e.target.value);reset();}}/></Field>
    <div className="reference-options">{files.map(f=><article className="reference-option" key={f.version_id}><strong>{f.name} · 固定版本 {f.seq}</strong><small>SHA-256 {f.sha256}</small>{f.scan_state==='waived'&&<small>此版本依据扫描例外放行，未标记为扫描通过。</small>}<button className="text-button" type="button" disabled={disabled} onClick={()=>{setFiles(current=>current.filter(v=>v.version_id!==f.version_id));reset();}}>移除此版本</button></article>)}</div>
    <button className="button secondary" type="button" disabled={disabled||files.length>=20} onClick={()=>setPicking(true)}>选择正式文件版本（{files.length}/20）</button>
    {picking&&<FileVersionPicker title="选择本次交付的准确文件版本" project={target.project_id} classification={classification} onClose={()=>setPicking(false)} onSelect={f=>{setFiles(current=>current.some(v=>v.version_id===f.version_id)?current:[...current,f]);setPicking(false);reset();}}/>}
    <Field label="本次提交原因"><input maxLength={200} value={reason} disabled={disabled} onChange={e=>{setReason(e.target.value);reset();}}/></Field>
    <label><input type="checkbox" checked={reviewed} disabled={disabled} onChange={e=>setReviewed(e.target.checked)}/>已核对准确文件、交付说明和原修订，明确允许秘书提交待验收</label>
    <button className="button secondary" type="button" disabled={disabled||!target.can_submit||!reviewed} onClick={()=>{try{onChange(makeDeliverableSubmission(target,files.map(f=>f.version_id),description,reason,reviewed));}catch(cause){setError(cause);}}}>将提交验收加入请求</button>
    {!target.can_submit&&<p className="form-note">当前交付不可提交，请核对状态与权限。</p>}
  </div>;
}
