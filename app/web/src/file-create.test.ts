import { describe, expect, it } from 'vitest';
import { makeFileCreate, fileCreateReceipt, type FileCreateView } from '@copypaste/contracts/file-update';
import type { Schemas } from './api';

const id=(n:number)=>'10000000-0000-4000-8000-'+String(n).padStart(12,'0');
const folder={id:id(1),version:3,project_id:id(2),state:'active'} as Schemas['Folder'];
const input=makeFileCreate(folder,'项目纪要.md','text/markdown',id(2));
const authorized={run_id:id(3),intent_sha256:'a'.repeat(64),target:{folder_id:id(1),folder_version:3,name:'项目纪要.md',media_type:'text/markdown',classification:'project_restricted'},status:'authorized',drafts:[],operation_id:null,result:null} as FileCreateView;
const run=(value:FileCreateView)=>({id:id(3),file_create:value}) as Schemas['Run'];

describe('explicit new-file scope and first formal version evidence',()=>{
  it('rejects cross-project, inactive folders and format/name ambiguity before sending',()=>{
    expect(input).toEqual({folder_id:id(1),expected_folder_version:3,name:'项目纪要.md',media_type:'text/markdown'});
    for(const value of [{...folder,project_id:null},{...folder,state:'archived'},{...folder,version:0}]) expect(()=>makeFileCreate(value as Schemas['Folder'],'记录.md','text/markdown',id(2))).toThrow();
    for(const name of ['../记录.md','记录.txt','记录.md ','记录\u0000.md']) expect(()=>makeFileCreate(folder,name,'text/markdown',id(2))).toThrow();
  });
  it('requires the accepted target and separates creation from updating a prior file',()=>{
    expect(fileCreateReceipt(run(authorized),input)).toBe(true);
    for(const target of [{...authorized.target,folder_id:id(9)},{...authorized.target,folder_version:4},{...authorized.target,name:'另一份.md'},{...authorized.target,media_type:'text/plain' as const}]) expect(fileCreateReceipt(run({...authorized,target}),input)).toBe(false);
    expect(fileCreateReceipt({...run(authorized),file_update:{} as Schemas['AgentFileUpdateView']},input)).toBe(false);
    expect(fileCreateReceipt(run(authorized))).toBe(false);
  });
  it('accepts only a scanned first version whose bytes and exact draft plan match',()=>{
    const committed={...authorized,status:'committed',operation_id:id(5),drafts:[{result_version_id:id(6),content_sha256:'b'.repeat(64),size_bytes:'123',plan_hash:'c'.repeat(64)}],result:{committed:true,file:{id:id(7),folder_id:input.folder_id,name:input.name,classification:'project_restricted',current_version_id:id(6)},file_version:{id:id(6),file_id:id(7),seq:1,base_version_id:null,source:'agent',scan_state:'clean',media_type:input.media_type,sha256:'b'.repeat(64),size_bytes:'123'}}} as unknown as FileCreateView;
    expect(fileCreateReceipt(run(committed),input)).toBe(true);
    for(const patch of [{seq:2},{base_version_id:id(8)},{sha256:'d'.repeat(64)},{scan_state:'waived'},{size_bytes:'124'}]) {
      const changed=structuredClone(committed); Object.assign(changed.result!.file_version,patch);expect(fileCreateReceipt(run(changed),input)).toBe(false);
    }
  });
});
