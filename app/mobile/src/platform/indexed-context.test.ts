import assert from 'node:assert/strict';
import { test as it } from 'node:test';
function expect(value:unknown){return {toBe:(other:unknown)=>assert.equal(value,other),toThrow:()=>assert.throws(value as ()=>unknown)};}
import { assertIndexedContext } from '../../../packages/contracts/src/indexed-context';
const file='10000000-0000-4000-8000-000000000001',version='10000000-0000-4000-8000-000000000002';
const view=()=>({conversation_id:file,conversation_revision_id:version,context_security_version:1,type:'file',id:file,version_id:version,resource_version:2,title:'资料.docx',state:'active',text:'香港🙂',offset:0,total_chars:5,next_offset:3,source_sha256:'c'.repeat(64),context_sha256:'d'.repeat(64),observed_at:'2026-10-28T00:00:00Z',untrusted_data:true,evidence:[{file_id:file,version_id:version,seq:2,name:'资料.docx',sha256:'a'.repeat(64),size_bytes:'512',media_type:'application/vnd.openxmlformats-officedocument.wordprocessingml.document',scan_state:'clean',scan_waiver_ref:null}],text_index:{extractor:'text-v1',state:'ready',source_sha256:'a'.repeat(64),text_sha256:'b'.repeat(64),indexed_at:'2026-10-28T00:00:00Z',total_characters:5,error_code:null}} as Parameters<typeof assertIndexedContext>[0]);
it('keeps raw-text compatibility and accepts exact-version derived Unicode chunks',()=>{
  expect(assertIndexedContext(view())).toBe(undefined);const raw=view();delete raw.text_index;expect(assertIndexedContext(raw)).toBe(undefined);
});
it('rejects evidence for another formal version, file or original hash',()=>{
  for(const change of [{version_id:file},{file_id:version},{sha256:'e'.repeat(64)},{scan_state:'quarantined'}]){const value=view();Object.assign(value.evidence[0],change);expect(()=>assertIndexedContext(value)).toThrow();}
});
it('requires a ready derivation with its own digest, time and total',()=>{
  for(const change of [{state:'pending'},{text_sha256:null},{indexed_at:'invalid'},{source_sha256:'bad'},{total_characters:6},{error_code:'FAILED'}]){const value=view();Object.assign(value.text_index!,change);expect(()=>assertIndexedContext(value)).toThrow();}
});
it('rejects malformed Unicode continuation and accepts the exact final chunk',()=>{
  for(const change of [{offset:-1},{next_offset:2},{next_offset:null},{offset:4}]){const value=view();Object.assign(value,change);expect(()=>assertIndexedContext(value)).toThrow();}
  const last=view();Object.assign(last,{offset:3,text:'核对',next_offset:null});expect(assertIndexedContext(last)).toBe(undefined);
});
