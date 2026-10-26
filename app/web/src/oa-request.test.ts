import { describe, expect, it } from 'vitest';
import { makeOADraft, makeOASubmit, oaRequestAccepted, oaRequestReceipt, type OABusiness, type OARequestView } from '@copypaste/contracts/oa-request';
import type { Schemas } from './api';
const id=(n:number)=>'10000000-0000-4000-8000-'+String(n).padStart(12,'0');
const b={id:id(1),title:'Current purchase',version:3,current_revision_id:id(2),content_hash:'a'.repeat(64),project_id:id(3),request_id:null,can_prepare_request:true,lifecycle:'draft',approval_state:'not_submitted'} as OABusiness;
const input=makeOADraft('purchase',b,undefined,b.project_id,'本人核对的准确版本');
const before={business:{...b,kind:'purchase'}} as unknown as OARequestView['before'];
const authorized={run_id:id(4),tool:'draft_request',kind:'purchase',business_id:b.id,request_id:null,arguments:input.arguments,before,intent_sha256:'b'.repeat(64),status:'authorized',operation_id:null,result:null} as OARequestView;
const run=(v:OARequestView)=>({id:id(4),actor_id:id(5),oa_request:v}) as Schemas['Run'];
const draft={id:id(6),version:1,kind:'purchase',business_id:b.id,applicant_id:id(5),project_id:b.project_id,state:'draft',active_instance_id:null,payload:{business_id:b.id,business_revision_id:b.current_revision_id,business_content_hash:b.content_hash}} as Schemas['Request'];
describe('exact OA authority, human preview and actual business evidence',()=>{
 it('rejects cross-project and immutable business targets before sending',()=>{
  for(const target of [{...b,project_id:null},{...b,can_prepare_request:false},{...b,lifecycle:'requested'},{...b,content_hash:'bad'}])expect(()=>makeOADraft('purchase',target as OABusiness,undefined,b.project_id,'核对')).toThrow();
  expect(()=>makeOADraft('purchase',b,undefined,b.project_id,' ')).toThrow();
 });
 it('accepts an authorized exact intent while keeping it separate from a business commit',()=>{
  expect(oaRequestAccepted(run(authorized),input)).toBe(true);expect(oaRequestReceipt(run(authorized))).toBe(false);
  expect(oaRequestAccepted(run({...authorized,arguments:{...input.arguments,reason:'改写'}}),input)).toBe(false);
  expect(oaRequestAccepted({...run(authorized),file_create:{} as Schemas['AgentFileCreateView']},input)).toBe(false);
  expect(oaRequestAccepted(run(authorized))).toBe(false);
 });
 it('validates a real draft against actor, immutable business revision and request version',()=>{
  const v={...authorized,status:'committed',operation_id:id(8),result:draft} as OARequestView;expect(oaRequestReceipt(run(v),input)).toBe(true);
  for(const result of [{...draft,applicant_id:id(9)},{...draft,version:9},{...draft,business_id:id(9)},{...draft,payload:{...draft.payload,business_revision_id:id(9)}},{...draft,state:'approved'}])expect(oaRequestReceipt(run({...v,result} as OARequestView),input)).toBe(false);
 });
 it('requires the reviewed current preview and does not silently accept a changed rule hash',()=>{
  const current={...b,request_id:draft.id},r={...draft,can_submit:true,current_revision_id:id(10),content_hash:'c'.repeat(64)} as Schemas['Request'];
  const p={request_id:r.id,request_version:r.version,revision_id:r.current_revision_id,content_hash:r.content_hash,preview_hash:'d'.repeat(64),payload:draft.payload,workflow:{id:id(11),steps:[{seq:1,assigned_user_id:id(12)}]}} as Schemas['SubmissionPreview'];
  const send=makeOASubmit('purchase',current,r,p,b.project_id,'本人确认');expect(send.tool).toBe('submit_request');
  for(const wrong of [{...p,request_version:2},{...p,content_hash:'e'.repeat(64)},{...p,payload:{...p.payload,business_revision_id:id(9)}}])expect(()=>makeOASubmit('purchase',current,r,wrong,b.project_id,'核对')).toThrow();
 });
 it('counts submission as committed only when a real instance has an active assigned review step',()=>{
  const args={request_id:draft.id,expected_version:2,revision_id:id(10),content_hash:'c'.repeat(64),workflow_definition_id:id(11),preview_hash:'d'.repeat(64),reason:'送审'};
  const result={...draft,version:3,current_revision_id:id(13),state:'in_review',active_instance_id:id(14),workflow:{id:id(11)},steps:[{request_id:draft.id,revision_id:id(13),instance_id:id(14),state:'active',assigned_user_id:id(12)}]} as Schemas['Request'];
  const v={...authorized,tool:'submit_request',request_id:draft.id,arguments:args,status:'committed',operation_id:id(8),result} as OARequestView;expect(oaRequestReceipt(run(v))).toBe(true);
  for(const wrong of [{...result,state:'approved'},{...result,active_instance_id:null},{...result,steps:[]},{...result,steps:[{...result.steps[0],instance_id:id(99)}]}])expect(oaRequestReceipt(run({...v,result:wrong} as OARequestView))).toBe(false);
 });
});
