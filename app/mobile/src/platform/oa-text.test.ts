import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
function expect(value:unknown){return {toBe:(other:unknown)=>assert.equal(value,other),toEqual:(other:unknown)=>assert.deepEqual(value,other),toThrow:()=>assert.throws(value as ()=>unknown)};}
import { makeOAText, makeOATextAdopt, oaTextAccepted, oaTextProven } from '../../../packages/contracts/src/oa-text';
import { oaRequestAccepted, type OABusiness, type OARequestView } from '../../../packages/contracts/src/oa-request';
import type { components } from '../../../packages/contracts/src/api';
type S=components['schemas'];
const id=(n:number)=>'10000000-0000-4000-8000-'+String(n).padStart(12,'0');
const b:OABusiness&{can_edit:boolean}={id:id(1),title:'Synthetic purchase',version:3,current_revision_id:id(2),content_hash:'a'.repeat(64),project_id:id(3),request_id:null,can_prepare_request:true,can_edit:true,lifecycle:'draft',approval_state:'not_submitted'};
const input=makeOAText('purchase',b,b.project_id,['purpose','notes'],'Draft the selected prose');
const before={business:{...b,kind:'purchase',classification:'project_restricted',payload:{content:{title:b.title,purpose:'Original purpose',notes:'',currency:'HKD',lines:[{id:id(8),unit_price:'12.34'}]}}}} as unknown as OARequestView['before'];
const authorized={run_id:id(4),tool:'draft_business_text',kind:'purchase',business_id:b.id,request_id:null,arguments:input.arguments,before,intent_sha256:'b'.repeat(64),status:'authorized',operation_id:null,result:null} as OARequestView;
const run=(v:OARequestView)=>({id:id(4),actor_id:id(5),oa_request:v}) as S['Run'];
const proposed=()=>({...authorized,status:'drafted',text_draft:{fields:{purpose:'Proposed purpose',notes:'Reviewed notes'},draft_sha256:'c'.repeat(64),source_refs:[{type:'agent_oa_request',id:authorized.run_id,resource_version:1,sha256:authorized.intent_sha256}],operation_id:null,result:null}} as OARequestView);
const adopted=()=>{const v=proposed();return {...v,status:'adopted',text_draft:{...v.text_draft!,operation_id:id(6),result:{...before.business,version:4,current_revision_id:id(7),content_hash:'d'.repeat(64),payload:{...before.business.payload,content:{...before.business.payload.content as object,...v.text_draft!.fields}}}}} as unknown as OARequestView;};
describe('selected OA prose, immutable suggestions and human adoption',()=>{
 it('requires the current editable business and selected prose fields',()=>{
  for(const change of [{can_edit:false},{project_id:null},{lifecycle:'active'},{approval_state:'in_review'},{content_hash:'bad'},{version:0}])expect(()=>makeOAText('purchase',{...b,...change} as typeof b,b.project_id,['notes'],'review')).toThrow();
  for(const fields of [[],['amount'],['notes','notes'],['renewal_terms']])expect(()=>makeOAText('purchase',b,b.project_id,fields,'review')).toThrow();
  expect(()=>makeOAText('contract',b,b.project_id,['purpose'],'review')).toThrow();
  expect(()=>makeOAText('purchase',b,b.project_id,['notes'],' ')).toThrow();
 });
 it('accepts the exact authorized target including array values',()=>{
  expect(oaTextAccepted(run(authorized),input)).toBe(true);
  expect(oaRequestAccepted(run({...authorized,arguments:structuredClone(input.arguments)}),input)).toBe(true);
  const changed={...authorized,arguments:{...input.arguments,fields:['notes']}} as OARequestView;
  expect(oaTextAccepted(run(changed),input)).toBe(false);
  expect(oaTextAccepted({...run(authorized),post_write:{} as S['AgentPostWriteView']},input)).toBe(false);
 });
 it('keeps proposed prose separate from any business write',()=>{
  expect(oaTextProven(proposed())).toBe(true);
  expect(oaTextAccepted(run(proposed()),input)).toBe(true);
  const v=proposed();v.text_draft!.fields.amount='999';expect(oaTextProven(v)).toBe(false);
 });
 it('rejects missing, blank, oversized and wrong-kind fields',()=>{
  for(const fields of [{notes:'only one'}, {purpose:' ',notes:'notes'}, {purpose:'x'.repeat(4001),notes:'notes'}, {purpose:'purpose',notes:'x'.repeat(20001)}]){const v=proposed();v.text_draft!.fields=fields as Record<string,string>;expect(oaTextProven(v)).toBe(false);}
 });
 it('requires explicit review and a bounded adoption reason',()=>{
  expect(()=>makeOATextAdopt(proposed(),false,'human review')).toThrow();
  expect(()=>makeOATextAdopt(proposed(),true,' ')).toThrow();
  expect(makeOATextAdopt(proposed(),true,' Human review ')).toEqual({draft_sha256:'c'.repeat(64),reviewed:true,reason:'Human review'});
 });
 it('verifies a new business revision with every other commercial fact preserved',()=>{
  expect(oaTextProven(adopted())).toBe(true);
  for(const change of [{version:3},{project_id:null},{current_revision_id:b.current_revision_id}]){const v=adopted();Object.assign(v.text_draft!.result!,change);expect(oaTextProven(v)).toBe(false);}
  const v=adopted();(v.text_draft!.result!.payload.content as Record<string,unknown>).currency='USD';expect(oaTextProven(v)).toBe(false);
 });
 it('cannot adopt the same suggestion twice or claim a receipt without an operation',()=>{
  expect(()=>makeOATextAdopt(adopted(),true,'again')).toThrow();
  const v=adopted();v.text_draft!.operation_id=null;expect(oaTextProven(v)).toBe(false);
 });
});
