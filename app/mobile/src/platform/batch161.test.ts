import test from 'node:test';
import assert from 'node:assert/strict';
import type { Schemas } from './session';
import { notificationAuthorization,notificationFragment,scopeAuthorization,fixedDeliveryAccepted } from '../../../packages/contracts/src/fixed-deliveries';
import { historyCommitAccepted,exactScanReport } from './file-operations';
import { taskInventoryKinds,inventoryURL } from '../../../packages/contracts/src/work-inventory';
const id=(v:number)=>'00000000-0000-4000-8000-'+String(v).padStart(12,'0');
test('notification selection counts Unicode code points, rejects truncation and preserves exact publication identity',()=>{
 const preview={request_id:id(1),run_id:id(2),run_version:3,body:'甲😀乙丙',body_sha256:'a'.repeat(64),total_chars:4} as Schemas['TelegramDisclosurePreview'];
 assert.equal(notificationFragment(preview,'1','2'),'😀乙');
 for(const [start,max] of [['1x','2'],['-1','2'],['1','4'],['0','0'],['9007199254740992','1']])assert.throws(()=>notificationFragment(preview,start,max));
 assert.throws(()=>notificationFragment({...preview,total_chars:5},'1','2'));assert.throws(()=>notificationAuthorization(preview,'1','2',false));
 const input=notificationAuthorization(preview,'1','2',true);assert.equal(input.notification?.body_sha256,preview.body_sha256);assert.equal(input.notification?.expected_version,3);
 const run={id:id(5),fixed_delivery:{run_id:id(5),kind:'notification',input:input.notification}} as unknown as Schemas['Run'];
 assert.equal(fixedDeliveryAccepted(run,input as Schemas['RunCreate']),true);
 assert.equal(fixedDeliveryAccepted({...run,fixed_delivery:{...run.fixed_delivery!,input:{...input.notification!,start_char:0}}},input as Schemas['RunCreate']),false);
});
test('scope authorization rejects stale, blocked, empty, expired or oversized full inventories',()=>{
 const p={id:id(1),version:4,state:'pending',can_execute:true,current_basis_matches:true,expires_at:'2099-01-01T00:00:00Z',plan_hash:'a'.repeat(64),snapshot:{items:[{}]}} as Schemas['ScopeTransfer'];
 assert.equal(scopeAuthorization(p,'exact reviewed change',true).scope_transfer?.plan_hash,p.plan_hash);
 for(const patch of [{state:'executed'},{can_execute:false},{current_basis_matches:false},{expires_at:'2000-01-01T00:00:00Z'},{snapshot:{items:[]}},{snapshot:{items:Array(101).fill({})}}])assert.throws(()=>scopeAuthorization({...p,...patch} as Schemas['ScopeTransfer'],'reviewed',true));
 assert.throws(()=>scopeAuthorization(p,'reviewed',false));
});
test('history promotion receipt binds new content identity, current file head and original source bytes',()=>{
 const run={file_id:id(1),source_version_id:id(2),sha256:'a'.repeat(64),size_bytes:'10',base_seq:2} as unknown as Schemas['HistoryRestore'];
 const value={committed:true,file:{id:id(1),current_version_id:id(3)},file_version:{id:id(3),file_id:id(1),sha256:run.sha256,size_bytes:'10',seq:3}} as Schemas['FileCommitResult'];
 assert.equal(historyCommitAccepted(value,run),true);
 for(const patch of [{sha256:'b'.repeat(64)},{size_bytes:'11'},{id:run.source_version_id},{seq:2},{file_id:id(9)}])assert.equal(historyCommitAccepted({...value,file_version:{...value.file_version,...patch}},run),false);
 const source={id:id(4),scan_result_id:id(5),sha256:run.sha256,size_bytes:'10'};const report={id:id(5),upload_id:id(4),sha256:run.sha256,size_bytes:'10'} as unknown as Schemas['UploadScanReport'];
 assert.equal(exactScanReport(report,source),true);assert.equal(exactScanReport({...report,upload_id:id(8)},source),false);
});
test('all four task inventories open the ordinary exact task route',()=>{
 assert.equal(taskInventoryKinds.length,4);
 for(const kind of taskInventoryKinds){const row={id:id(2),project_id:id(1)} as Schemas['WorkInventoryItem'];assert.ok(inventoryURL(kind,row,'mobile'));}
});
