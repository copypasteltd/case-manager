import assert from 'node:assert/strict';
import { checkedRetirementPlan, checkedCompletedDraftRetirementPlan as checked, checkedRetirementScope, completedDraftRetirementCreate as create, retirementCreate, retirementDecision, retirementRenewal, retirementCandidateLabel, type RetirementPlan } from '../src/object-retirement';
import { retirementPlan, retirementID as id, retirementNow as now } from './object-retirement-fixture';

export function completedRetirementPlan(replacement=false): RetirementPlan {
 const p=retirementPlan();
 p.snapshot={...p.snapshot,kind:'uncommitted_draft',file_id:replacement?id(3):undefined,file_version:replacement?3:0,seq:0,folder_id:id(20),folder_version:2,upload_version:7,upload_state:'ready',object_count:2,policy:{...p.snapshot.policy!,kind:'draft'}};
 p.items=p.items.map((item,i)=>({...item,provider:'oss',role:i===0?'upload_draft':'promotion_candidate'}));
 return p;
}

export function completedRetirementChecks(test:(name:string,fn:()=>void)=>void){
 test('new completed uploads use folder authority without inventing a formal file identity',()=>{
  const p=completedRetirementPlan();assert.equal(checked(p,id(20)),p);assert.throws(()=>checked(p,id(3)));assert.throws(()=>checkedRetirementPlan(p,id(20)));assert.equal(p.snapshot.file_version,0);assert.equal(p.snapshot.file_id,undefined);
 });
 test('replacement completed uploads retain file identity but use the exact folder workspace',()=>{
  const p=completedRetirementPlan(true);assert.equal(checked(p,id(20)),p);assert.throws(()=>checkedRetirementPlan(p,id(3)));assert.throws(()=>checkedRetirementScope({...p,snapshot:{...p.snapshot,folder_id:id(21)}},p));assert.match(retirementCandidateLabel(p.snapshot),/替换上传/);
 });
 test('multiple known objects may share a provider but not identity, ordinal or source role',()=>{
  const p=completedRetirementPlan();assert.equal(checked(p,id(20)).items.length,2);assert.throws(()=>checked({...p,items:[p.items[0],{...p.items[1],id:p.items[0].id}]},id(20)));assert.throws(()=>checked({...p,items:[p.items[1],p.items[0]]},id(20)));assert.throws(()=>checked({...p,items:[p.items[0],{...p.items[1],role:'upload_draft'}]},id(20)));
 });
 test('the fixed object count and completed upload revisions are mandatory',()=>{
  const p=completedRetirementPlan();for(const patch of [{object_count:1},{object_count:65},{upload_version:0},{folder_version:0},{seq:1},{file_version:1}])assert.throws(()=>checked({...p,snapshot:{...p.snapshot,...patch}},id(20)));
 });
 test('creation pins both revisions and cannot use the formal-file creation command',()=>{
  const c=completedRetirementPlan().snapshot;assert.deepEqual(create(c,' review '),{expected_folder_version:2,expected_upload_version:7,target_id:c.target_id,reason:'review'});assert.throws(()=>retirementCreate(c,'review'));assert.throws(()=>create({...c,eligible:false},'review'));assert.throws(()=>create({...c,kind:'history'},'review'));
 });
 test('completed upload approval keeps the human observation and irreversible acknowledgement',()=>{
  const p=completedRetirementPlan();assert.equal(retirementDecision(p,'review',true,'approve',now).snapshot_sha256,p.snapshot_sha256);assert.throws(()=>retirementDecision(p,'review',false,'approve',now));assert.throws(()=>retirementDecision(p,'review',true,'approve',0));assert.throws(()=>checked({...p,state:'completed'},id(20)));
 });
 test('a later grant selects every stopped object including more than two items',()=>{
  const p=completedRetirementPlan();p.state='approved';p.snapshot.object_count=4;p.items=[...p.items,...p.items.map((item,i)=>({...item,id:id(30+i),ordinal:i+2,role:'promotion_candidate' as const}))].map(item=>({...item,state:'blocked'}));
  const renewal=retirementRenewal(p,'adopt all four',true);assert.equal(renewal.items.length,4);assert.deepEqual(renewal.items.map(v=>v.id),p.items.map(v=>v.id));
 });
}
