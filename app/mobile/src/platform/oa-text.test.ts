import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
function expect(value:unknown){return {toBe:(other:unknown)=>assert.equal(value,other),toEqual:(other:unknown)=>assert.deepEqual(value,other),toThrow:()=>assert.throws(value as ()=>unknown)};}
import { makeOAText, makeOATextAdopt, oaTextAccepted, oaTextProven, oaTextFields, oaTextName, oaTextOriginal } from '../../../packages/contracts/src/oa-text';
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


describe('exact selected purchase/claim line descriptions',()=>{
 const first=id(8),second=id(9),field='line:'+first+':description';
 const content={title:'Synthetic purchase',purpose:'Original',notes:'',currency:'HKD',lines:[{id:first,description:'Same description',quantity:'2',unit:'licence',unit_price:'12.34',tax_amount:'1.23',receipt_version_ids:[id(10)],funding:'employee_advance'},{id:second,description:'Same description',unit_price:'7.89'}]};
 const business={...b,lines:content.lines};
 const view=()=>{const input=makeOAText('purchase',business,b.project_id,[field,'notes'],'Review this exact line');return {...authorized,arguments:input.arguments,before:{...before,business:{...before.business,payload:{...before.business.payload,content:structuredClone(content)}}},status:'drafted',text_draft:{...proposed().text_draft!,fields:{[field]:'Reviewed description',notes:'Reviewed notes'}}} as OARequestView;};
 const saved=()=>{const v=view();return {...v,status:'adopted',text_draft:{...v.text_draft!,operation_id:id(6),result:{...v.before.business,version:4,current_revision_id:id(7),content_hash:'d'.repeat(64),payload:{...v.before.business.payload,content:{...structuredClone(content),notes:'Reviewed notes',lines:content.lines.map((l,i)=>i===0?{...l,description:'Reviewed description'}:structuredClone(l))}}}}} as unknown as OARequestView;};
 it('discovers exact IDs with separate labels for identical descriptions',()=>{
  expect(oaTextFields('purchase',business)).toEqual(['purpose','notes',field,'line:'+second+':description']);
  expect(oaTextFields('contract',business)).toEqual(['renewal_terms','termination_terms','notes']);
  expect(oaTextName(field,content)).toBe('明细 1 · Same description');
  expect(oaTextOriginal(view(),field)).toBe('Same description');
  for(const selected of [['line:'+id(20)+':description'],['line:0:description'],['line:'+first+':unit_price'],[field,field]])expect(()=>makeOAText('purchase',business,b.project_id,selected,'review')).toThrow();
  expect(()=>makeOAText('contract',business,b.project_id,[field],'review')).toThrow();
 });
 it('accepts selected line prose and its exact human adoption receipt',()=>{
  expect(oaTextProven(view())).toBe(true);expect(oaTextProven(saved())).toBe(true);
  expect(oaRequestAccepted(run(view()),{tool:'draft_business_text',arguments:view().arguments} as S['AgentOARequestInput'])).toBe(true);
  expect(makeOATextAdopt(view(),true,'Reviewed original and proposal').reviewed).toBe(true);
 });
 it('rejects unknown, duplicate, NUL, noncanonical or overlong line output',()=>{
  for(const text of ['', ' ', 'x'.repeat(1001),'bad\0text',' padding ']){const v=view();v.text_draft!.fields[field]=text;expect(oaTextProven(v)).toBe(false);}
  const unknown=view();unknown.text_draft!.fields['line:'+id(21)+':description']='Unknown';delete unknown.text_draft!.fields[field];expect(oaTextProven(unknown)).toBe(false);
  const repeated=view();(repeated.arguments as S['AgentOATextArguments']).fields=[field,field];expect(oaTextProven(repeated)).toBe(false);
 });
 it('rejects any different commercial fact, unselected line, identity or order',()=>{
  for(const key of ['unit_price','quantity','unit','tax_amount','receipt_version_ids','funding','id']){const v=saved();const body=v.text_draft!.result!.payload.content as typeof content;(body.lines[0] as unknown as Record<string,unknown>)[key]='changed';expect(oaTextProven(v)).toBe(false);}
  const other=saved();(other.text_draft!.result!.payload.content as typeof content).lines[1].description='Unselected change';expect(oaTextProven(other)).toBe(false);
  const reordered=saved();(reordered.text_draft!.result!.payload.content as typeof content).lines.reverse();expect(oaTextProven(reordered)).toBe(false);
 });
});
