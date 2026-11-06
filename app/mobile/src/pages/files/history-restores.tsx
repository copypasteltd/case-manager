import { Text, View } from '@tarojs/components';
import { useState } from 'react';
import { historyCommitAccepted } from '../../platform/file-operations';
import { useCollection } from '../../platform/collection';
import { useCommand, CommandNotice, MFAConfirmation } from '../../platform/command';
import type { Schemas } from '../../platform/session';
import { readVersion } from '../../platform/page';
import { Action, Check, Empty, Fact, Notice, Section, bytes, date } from '../../ui';
import { TextField } from '../business/controls';
type Restore = Schemas['HistoryRestore'];
const states: Record<Restore['state'], string> = { queued:'等待复制', copying:'复制历史内容中', verifying:'完整校验中', retry_wait:'等待服务端重试', ready:'待本人正式提交', committed:'已生成新正式版本', cancelled:'已取消', failed:'恢复未完成' };
export function RestoreHistoricalVersion({ file, version, committed }: { file:Schemas['File']; version:Schemas['FileVersion']; committed():void }) {
 const [open,setOpen]=useState(false);
 if(!file.can_restore_history || version.id===file.current_version_id)return null;
 return <>{!open?<Action secondary disabled={version.availability!=='available'||!['clean','waived'].includes(version.scan_state)} onClick={()=>setOpen(true)}>将内容版本 {version.seq} 恢复为新版本</Action>:<RestoreCreate key={file.id+':'+file.version+':'+version.id} file={file} source={version} close={()=>setOpen(false)} committed={()=>{setOpen(false);committed();}}/>}</>;
}
function RestoreCreate({file,source,close,committed}:{file:Schemas['File'];source:Schemas['FileVersion'];close():void;committed():void}) {
 const [reason,setReason]=useState(''),[fresh,setFresh]=useState(false),[confirmed,setConfirmed]=useState(false);
 const command=useCommand<Restore>(committed,'history-create:'+file.id+':'+file.version+':'+source.id,v=>v.file_id===file.id&&v.source_version_id===source.id&&v.base_version_id===file.current_version_id&&v.sha256===source.sha256&&v.size_bytes===source.size_bytes);
 return <View className="cp-pad"><Notice error={command.error}/><CommandNotice blocked={command.blocked}/><Text>复制固定历史内容为独立候选；完成校验后仍需再次明确提交。</Text><Fact label={'来源内容版本 '+source.seq}><Text>{bytes(source.size_bytes)}</Text><Text selectable>{source.sha256}</Text></Fact>{source.scan_state==='waived'&&<Text className="cp-notice">此来源经例外放行，恢复不代表重新通过安全扫描。</Text>}<TextField name="history-reason" label="恢复原因" value={reason} onChange={setReason} disabled={command.busy} max={200}/><Check checked={confirmed} onChange={setConfirmed} disabled={command.busy}>我已核对准确来源与当前基础版本。</Check><MFAConfirmation onFresh={setFresh} disabled={command.busy}/><Action disabled={!fresh||!confirmed||!reason.trim()||command.blocked} loading={command.busy} onClick={()=>void command.submit('/files/'+file.id+'/history-restores',{expected_version:file.version,source_version_id:source.id,reason})}>创建恢复候选</Action><Action secondary disabled={command.busy} onClick={close}>返回</Action></View>;
}
export function HistoryRestores({file,committed}:{file:Schemas['File'];committed():void}) {
 const list=useCollection<Restore>(file.can_view_history_restores?'/files/'+file.id+'/history-restores?limit=20':null);
 if(!file.can_view_history_restores)return null;
 // Mount one settled inventory only; loading snapshots never retain an actionable panel.
 if(list.loading)return null;
 return <Section title="我的历史内容恢复"><Notice error={list.error}/>{!list.error&&list.items.length?list.items.map(run=><RestoreCard key={run.id+':'+run.version} run={run} canCommit={!!file.can_restore_history} refreshed={()=>{list.reload();committed();}}/>):!list.error&&<Empty>尚无本人恢复任务。请在准确历史版本上创建。</Empty>}{list.next&&<Action secondary loading={list.moreBusy} onClick={()=>void list.more()}>继续读取恢复任务</Action>}<Action secondary loading={list.loading} onClick={list.reload}>刷新复制、扫描与校验进度</Action></Section>;
}
function RestoreCard({run,canCommit,refreshed}:{run:Restore;canCommit:boolean;refreshed():void}) {
 const [action,setAction]=useState<'commit'|'cancel'>(),[reason,setReason]=useState(''),[fresh,setFresh]=useState(false),[confirmed,setConfirmed]=useState(false);
 const command=useCommand<Restore|Schemas['FileCommitResult']>(refreshed,'history-action:'+run.id+':'+run.version,v=>action==='cancel'?'id' in v&&v.id===run.id&&v.state==='cancelled':'file' in v&&historyCommitAccepted(v,run));
 const expired=Date.parse(run.expires_at)<=Date.now(),finished=['committed','cancelled','failed'].includes(run.state);
 return <View className="cp-pad"><Fact label={states[run.state]}><Text>来源版本 {run.source_seq} · 固定基础版本 {run.base_seq}</Text><Text>复制 {bytes(run.copied_bytes)} / {bytes(run.size_bytes)} · 校验 {bytes(run.verified_bytes)}</Text><Text>{run.reason} · 到期 {date(run.expires_at)}</Text><Text selectable>{run.sha256}</Text>{run.error_code&&<Text>执行记录 {run.error_code}</Text>}{expired&&!finished&&<Text>任务已到期，请读取最新状态并重新创建候选。</Text>}</Fact>{run.result_version_id&&<Action secondary onClick={()=>readVersion(run.file_id,run.result_version_id!)}>查阅已生成的准确正式版本</Action>}{!action&&<>{run.state==='ready'&&!expired&&canCommit&&<Action onClick={()=>setAction('commit')}>核对并生成新的正式版本</Action>}{!finished&&<Action secondary onClick={()=>setAction('cancel')}>取消这个恢复任务</Action>}</>}{action&&<><Notice error={command.error}/><CommandNotice blocked={command.blocked}/>{action==='cancel'&&<TextField name="restore-cancel-reason" label="取消原因" value={reason} onChange={setReason} disabled={command.busy} max={200}/>}<Check checked={confirmed} onChange={setConfirmed} disabled={command.busy}>确认{action==='commit'?'使用上述固定基础版本与摘要提交':'停止此恢复任务并保留历史'}。</Check><MFAConfirmation onFresh={setFresh} disabled={command.busy}/><Action disabled={!confirmed||!fresh||command.blocked||action==='cancel'&&!reason.trim()} loading={command.busy} onClick={()=>void command.submit('/history-restores/'+run.id+'/'+action,action==='commit'?{expected_version:run.version,base_version_id:run.base_version_id,sha256:run.sha256}:{expected_version:run.version,reason})}>确认{action==='commit'?'提交':'取消'}</Action><Action secondary disabled={command.busy} onClick={()=>{setAction(undefined);setConfirmed(false);setFresh(false);}}>返回核对</Action></>}</View>;
}
