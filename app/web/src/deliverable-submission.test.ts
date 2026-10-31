import assert from 'node:assert/strict';
import { test } from 'vitest';
import { makeDeliverableSubmission, submissionFiles } from '@copypaste/contracts/deliverable-submission';
import { makeMutation, mutationReceipt, type MutationView } from '@copypaste/contracts/mutation';
import type { components } from '@copypaste/contracts';
const id=(n:number)=>'10000000-0000-4000-8000-'+String(n).padStart(12,'0');
const target={id:id(1),project_id:id(2),version:5,protocol_version:1,state:'draft',current_revision_id:null,can_submit:true} as components['schemas']['Deliverable'];
const input=()=>makeDeliverableSubmission(target,[id(3)],'  固定交付说明  ',' 人工确认 ',true);
const view=():MutationView=>({run_id:id(4),tool:'submit_deliverable',resource_type:'deliverable',resource_id:target.id,base_version:5,intent_sha256:'b'.repeat(64),arguments:input().arguments,before:{...target,submission_files:[{file_id:id(9),version_id:id(3),name:'binary.bin',seq:2,size_bytes:'120000000000',sha256:'a'.repeat(64),media_type:'application/octet-stream',scan_state:'clean',scan_waiver_ref:null}]},status:'committed',operation_id:id(5),result_version:6,result:{id:target.id,version:6,revision_id:id(6)}});
const ack=(m=view())=>mutationReceipt({id:id(4),mutation:m} as components['schemas']['Run'],input());
test('fixes original revision, ordered version IDs and explicit review',()=>{
 assert.deepEqual(input().arguments,{id:target.id,expected_version:5,expected_revision_id:null,file_version_ids:[id(3)],description:'固定交付说明',reason:'人工确认'});
 assert.throws(()=>makeDeliverableSubmission(target,[id(3)],'说明','原因',false));
 assert.throws(()=>makeMutation('submit_deliverable',target,'description','说明',false,'原因'));
 for(const versions of [[],[id(3),id(3)],['unknown'],Array.from({length:21},(_,i)=>id(i+10))])assert.throws(()=>makeDeliverableSubmission(target,versions,'说明','原因',true));
});
test('acknowledges only an actual new fixed revision with complete evidence',()=>{
 assert.equal(ack(),true);
 for(const patch of [{revision_id:null},{revision_id:'invented'},{id:id(8)},{version:7},{extra:true}]){const m=view();m.result={...m.result,...patch};assert.equal(ack(m),false)}
 const m=view();m.before.current_revision_id=id(7);assert.equal(ack(m),false);
 const missing=view();delete missing.before.submission_files;assert.equal(ack(missing),false);
});
test('keeps waiver metadata honest and rejects reordered or substituted file evidence',()=>{
 const m=view();const f=(m.before.submission_files as Record<string,unknown>[])[0];f.scan_state='waived';f.scan_waiver_ref=id(12);assert.equal(submissionFiles(m)[0].scan_state,'waived');assert.equal(ack(m),true);
 f.scan_waiver_ref=null;assert.equal(ack(m),false);f.scan_waiver_ref=id(12);f.version_id=id(13);assert.equal(ack(m),false);
});
test('preserves accepted revision history and refuses submitted or stale capabilities',()=>{
 const old=id(14);assert.equal(makeDeliverableSubmission({...target,state:'accepted',current_revision_id:old},[id(3)],'说明','原因',true).arguments.expected_revision_id,old);
 for(const patch of [{state:'submitted'},{can_submit:false},{current_revision_id:'guess'},{version:0}])assert.throws(()=>makeDeliverableSubmission({...target,...patch} as typeof target,[id(3)],'说明','原因',true));
});
