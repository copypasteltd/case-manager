import { Text, View } from '@tarojs/components';
import Taro from '@tarojs/taro';
import { useState } from 'react';
import { useCollection, useRecord } from '../../platform/collection';
import { client } from '../../platform/taro';
import { identifier, usePage } from '../../platform/page';
import { useCommand, CommandNotice, MFAConfirmation } from '../../platform/command';
import type { Schemas } from '../../platform/session';
import { Action, Busy, Check, Empty, Fact, Notice, Page, Row, Section, bytes, date } from '../../ui';
import { TextField } from '../business/controls';
type Waiver=Schemas['ScanWaiver'];
export function ScanEvidence({report}:{report:Schemas['UploadScanReport']}) {
 return <Fact label={'原始结论：'+({clean:'扫描通过',blocked:'已阻断',unscannable:'无法扫描'}[report.state])}><Text>{report.code} · 实际送检 {bytes(report.streamed_bytes)} / {bytes(report.size_bytes)}</Text><Text>引擎 {report.engine??'未提供'} · {report.engine_version??'未提供'} · 签名库 {report.signature_revision??'未提供'}</Text><Text selectable>{report.sha256}</Text><Text>例外批准不会改写原始扫描结论。</Text></Fact>;
}
export default function ScanPage(){
 const id=Taro.useRouter().params.id;
 const page=usePage(async scope=>id?client.request<Waiver>('/scan-waivers/'+identifier(id),{scope}):null);
 return <Page title="扫描报告与独立复核" home><Notice error={page.error}/>{page.loading?<Busy/>:page.result&&(page.result.value?<WaiverDetail key={page.result.value.id+':'+page.result.value.version+':'+page.result.me.authorization_version} value={page.result.value} me={page.result.me} refresh={()=>void page.reload()}/>:<WaiverList/>)}<Action secondary loading={page.loading} onClick={()=>void page.reload()}>刷新当前权限与申请</Action></Page>;
}
function WaiverList(){
 const list=useCollection<Waiver>('/scan-waivers?limit=20');
 return <Section title="本人申请与指定复核"><Text>仅列当前可读记录。恶意内容不能放行，例外须有明确制度与独立审核。</Text><Notice error={list.error}/>{list.loading?<Busy/>:!list.error&&list.items.length?list.items.map(v=><Row key={v.id} title={v.name} subtitle={v.state+' · '+date(v.created_at)} onClick={()=>void Taro.navigateTo({url:'/pages/scan/index?id='+v.id})}/>):!list.error&&<Empty>没有当前可读申请。</Empty>}{list.next&&<Action secondary loading={list.moreBusy} onClick={()=>void list.more()}>继续读取申请</Action>}</Section>;
}
function WaiverDetail({value:v,me,refresh}:{value:Waiver;me:Schemas['Me'];refresh():void}){
 const report=useRecord<Schemas['UploadScanReport']>('/scan-waivers/'+v.id+'/scan-report');
 const [decision,setDecision]=useState<'approve'|'reject'|'cancel'>(),[reason,setReason]=useState(''),[risk,setRisk]=useState(false),[fresh,setFresh]=useState(false);
 const command=useCommand<Waiver>(refresh,'scan-waiver:'+v.id+':'+v.version,result=>result.id===v.id&&result.version===v.version+1&&result.sha256===v.sha256&&result.scan_report_id===v.scan_report_id);
 const valid=!!report.value&&!report.error&&report.value.id===v.scan_report_id&&report.value.upload_id===v.upload_id&&report.value.sha256===v.sha256&&report.value.size_bytes===v.size_bytes;
 const pending=v.state==='pending'&&Date.parse(v.expires_at)>Date.now();
 return <Section title={v.name}><Fact label={'申请状态 '+v.state}><Text>制度修订 {v.policy_version} · {bytes(v.size_bytes)} · 有效至 {date(v.expires_at)}</Text><Text>{v.reason}</Text><Text selectable>{v.sha256}</Text>{v.decision_reason&&<Text>{v.decision_reason}</Text>}</Fact><Notice error={report.error}/>{report.loading?<Busy/>:report.value&&<ScanEvidence report={report.value}/>}<Notice error={command.error}/><CommandNotice blocked={command.blocked}/>{pending&&<View className="cp-pad">{v.owner_id===me.user.id?<Action secondary onClick={()=>setDecision('cancel')}>取消本人申请</Action>:v.reviewer_id===me.user.id&&me.actions.includes('file.scan.waive')&&<><Action secondary onClick={()=>setDecision('reject')}>拒绝例外</Action><Action disabled={!valid||report.value?.state!=='unscannable'} onClick={()=>setDecision('approve')}>审阅并例外放行</Action></>}{decision&&<><TextField name="waiver-decision-reason" label="处理原因" value={reason} onChange={setReason} disabled={command.busy} max={200}/>{decision==='approve'&&<Check checked={risk} onChange={setRisk} disabled={command.busy}>我已核对固定报告、制度与未完成安全扫描的风险。</Check>}<MFAConfirmation onFresh={setFresh} disabled={command.busy}/><Action disabled={!fresh||!reason.trim()||command.blocked||decision==='approve'&&(!risk||!valid||report.value?.state!=='unscannable')} loading={command.busy} onClick={()=>void command.submit('/scan-waivers/'+v.id+(decision==='cancel'?'/cancel':'/decisions'),decision==='cancel'?{expected_version:v.version,action:'cancel',reason}:{expected_version:v.version,upload_version:v.upload_version,scan_report_id:v.scan_report_id,sha256:v.sha256,decision,reason,acknowledged_risk:risk})}>确认{decision==='approve'?'例外放行':decision==='reject'?'拒绝':'取消'}</Action></>}</View>}</Section>;
}
