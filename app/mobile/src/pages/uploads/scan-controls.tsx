import { Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { useState } from 'react';
import { exactScanReport } from '../../platform/file-operations';
import type { Schemas } from '../../platform/session';
import type { Upload } from '../../platform/upload-transfer';
import { useRecord } from '../../platform/collection';
import { useCommand, CommandNotice, MFAConfirmation } from '../../platform/command';
import { Action, Busy, Check, Notice } from '../../ui';
import { TextField } from '../business/controls';
import { ScanEvidence } from '../scan';
export function UploadScanControls({upload:u,disabled,reload}:{upload:Upload;disabled:boolean;reload():void}){
 const [open,setOpen]=useState(false);
 return <><Action secondary disabled={disabled} onClick={()=>setOpen(!open)}>核对固定扫描报告与后续处理</Action>{open&&<Controls key={u.id+':'+u.version} upload={u} disabled={disabled} reload={reload}/>}</>;
}
function Controls({upload:u,disabled,reload}:{upload:Upload;disabled:boolean;reload():void}){
 const report=useRecord<Schemas['UploadScanReport']>(u.scan_result_id?'/uploads/'+u.id+'/scan-report':null);
 const [action,setAction]=useState<'rescan'|'waivers'>(),[reason,setReason]=useState(''),[risk,setRisk]=useState(false),[fresh,setFresh]=useState(false);
 const command=useCommand<Upload|Schemas['ScanWaiver']>(v=>{if(action==='waivers'&&'reviewer_id' in v)void Taro.navigateTo({url:'/pages/scan/index?id='+v.id});reload();},'upload-scan:'+u.id+':'+u.version,v=>action==='waivers'?'scan_report_id' in v&&v.upload_id===u.id&&v.scan_report_id===u.scan_result_id&&v.sha256===u.sha256:v.id===u.id&&v.version===u.version+1);
 const valid=!!report.value&&!report.error&&exactScanReport(report.value,u);
 return <View className="cp-pad"><Notice error={report.error??command.error}/><CommandNotice blocked={command.blocked}/>{report.loading?<Busy/>:report.value?<ScanEvidence report={report.value}/>:<Text>尚无固定扫描报告。</Text>}{!u.retirement_plan_id&&u.state==='blocked'&&<><Action secondary disabled={disabled||command.busy} onClick={()=>setAction('rescan')}>重新扫描同一固定源</Action>{valid&&report.value?.state==='unscannable'&&<Action secondary disabled={disabled||command.busy} onClick={()=>setAction('waivers')}>按现行制度申请例外复核</Action>}</>}{action&&<><Text>排队或申请成功不表示通过扫描。服务端重新核对制度、固定源、当前权限与状态。</Text>{action==='waivers'&&<><TextField name="scan-waiver-reason" label="例外申请原因" value={reason} onChange={setReason} disabled={disabled||command.busy} max={200}/><Check checked={risk} onChange={setRisk} disabled={disabled||command.busy}>已核对固定报告并知悉未完成安全扫描。</Check></>}<MFAConfirmation onFresh={setFresh} disabled={disabled||command.busy}/><Action disabled={disabled||!fresh||command.blocked||action==='waivers'&&(!reason.trim()||!risk||!valid)} loading={command.busy} onClick={()=>void command.submit('/uploads/'+u.id+'/'+action,action==='rescan'?{expected_version:u.version}:{expected_version:u.version,scan_report_id:u.scan_result_id,sha256:u.sha256,reason,acknowledged_risk:risk})}>确认{action==='rescan'?'安排扫描':'提交独立复核'}</Action></>}</View>;
}
