import { describe, expect, it } from 'vitest';
import type { Schemas } from './api';
import { makePostDraft, makePostPublish, postWriteAccepted, postWriteReceipt } from '@copypaste/contracts/post-write';
const uuid=(n:number)=>`12345678-1234-1234-1234-${String(n).padStart(12,'0')}`;
function fixture(){
 const p={id:uuid(1),version:4,project_id:uuid(2),author_id:uuid(3),type:'project_log',title:'Reviewed article',classification:'project_restricted',body:'Full exact body',summary:'Summary',tags:['verified'],source_refs:[],state:'published',current_revision_id:uuid(4),content_hash:'a'.repeat(64),published_revision_id:uuid(5),has_unpublished_changes:true,can_edit:true,can_publish:true} satisfies Schemas['Post'];
 return p;
}
function runBase(actor_id:string): Omit<Schemas['Run'],'post_write'> {
 return {id:uuid(20),version:1,conversation_id:uuid(30),actor_id,profile_id:uuid(31),client_message_id:uuid(32),state:'running',last_event_seq:2,can_cancel:true,cancel_requested_at:null,expires_at:'2027-01-03T10:00:00Z'};
}
function basis(p:ReturnType<typeof fixture>): Schemas['AgentPostWriteBasis'] {
 return {scope:{project_id:p.project_id,version:3,state:'active',classification:'project_restricted'},post:p,attachments:[]};
}

describe('knowledge write authorization and evidence',()=>{
 it('fixes the complete base and attachment versions, rejecting floating or wrong targets',()=>{
  const p=fixture(),r={type:'file',id:uuid(10),version_id:uuid(11)};
  const v=makePostDraft(p.project_id,p.type!,p.title,p.classification!,[r],' Generate draft ',p);
  expect(v).toMatchObject({tool:'draft_post',post_id:p.id,expected_version:p.version,base_revision_id:p.current_revision_id,content_hash:p.content_hash,source_refs:[r],reason:'Generate draft'});
  expect(()=>makePostDraft(null,p.type!,p.title,p.classification!,[],'reason',p)).toThrow();
  expect(()=>makePostDraft(p.project_id,'sop',p.title,p.classification!,[],'reason',p)).toThrow();
  expect(()=>makePostDraft(p.project_id,p.type!,p.title,p.classification!,[{type:'file',id:uuid(10)}],'reason')).toThrow();
 });
 it('publication requires publish authority, complete body and a changed revision',()=>{
  const p=fixture();expect(makePostPublish(p,p.project_id,'Review complete')).toEqual({tool:'publish_post',post_id:p.id,expected_version:4,base_revision_id:p.current_revision_id,content_hash:p.content_hash,reason:'Review complete'});
  for(const patch of [{can_publish:false},{body:''},{has_unpublished_changes:false},{state:'archived' as const}])expect(()=>makePostPublish({...p,...patch},p.project_id,'reason')).toThrow();
 });
 it('authorization receipt must retain the exact intended object and inputs',()=>{
  const p=fixture(),input=makePostPublish(p,p.project_id,'reviewed'),run={...runBase(p.author_id),post_write:{run_id:uuid(20),input,before:basis(p),intent_sha256:'b'.repeat(64),status:'authorized',operation_id:null,result:null}} satisfies Schemas['Run'];
  expect(postWriteAccepted(run,input)).toBe(true);expect(postWriteAccepted(run,{...input,reason:'changed'})).toBe(false);expect(postWriteAccepted({...run,oa_request:{} as Schemas['AgentOARequestView']},input)).toBe(false);expect(postWriteReceipt(run)).toBe(false);
 });
 it('publication proves only the exact reviewed revision and preserves content digest',()=>{
  const p=fixture(),input=makePostPublish(p,p.project_id,'reviewed'),result={...p,version:5,published_revision_id:p.current_revision_id,has_unpublished_changes:false},run={...runBase(p.author_id),post_write:{run_id:uuid(20),input,before:basis(p),intent_sha256:'b'.repeat(64),status:'committed',operation_id:uuid(21),result}} satisfies Schemas['Run'];
  expect(postWriteReceipt(run)).toBe(true);expect(postWriteReceipt({...run,post_write:{...run.post_write!,result:{...result,content_hash:'c'.repeat(64)}}})).toBe(false);
  expect(postWriteReceipt({...run,post_write:{...run.post_write!,result:{...result,published_revision_id:uuid(22)}}})).toBe(false);
 });
 it('revising an existing published article cannot silently replace the published revision',()=>{
  const p=fixture(),input=makePostDraft(p.project_id,p.type!,p.title,p.classification!,[],'rewrite',p),result={...p,version:5,current_revision_id:uuid(22)},run={...runBase(p.author_id),post_write:{run_id:uuid(20),input,before:basis(p),intent_sha256:'b'.repeat(64),status:'committed',operation_id:uuid(21),result}} satisfies Schemas['Run'];
  expect(postWriteReceipt(run)).toBe(true);expect(postWriteReceipt({...run,post_write:{...run.post_write!,result:{...result,published_revision_id:result.current_revision_id}}})).toBe(false);
 });
});
