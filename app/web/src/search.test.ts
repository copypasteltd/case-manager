import { expect, it } from 'vitest';
import { checkedSearch, checkedFileText, searchPath, fileTextPath, type SearchHit, type FileText } from '@copypaste/contracts/search';
const file='10000000-0000-4000-8000-000000000001', version='10000000-0000-4000-8000-000000000002', time='2026-10-28T00:00:00Z';
const hit=():SearchHit=>({ resource:{type:'file',id:file,version_id:version},title:'交付说明.md',snippet:'香港正文🙂',updated_at:time,project_id:null,match_types:['body'],text_state:'ready',indexed_at:time });
const text=():FileText=>({ file_id:file,version_id:version,extractor:'text-v1',state:'ready',source_sha256:'a'.repeat(64),text_sha256:'b'.repeat(64),indexed_at:time,error_code:null,total_characters:7,offset:0,text:'香港正文🙂',next_offset:5 });
it('encodes literal search text and only named resource types',()=>{
  expect(searchPath(' 100%_香港\\ ', 'file')).toBe('/search?q=100%25_%E9%A6%99%E6%B8%AF%5C&limit=20&type=file');
  for(const q of ['', 'x'.repeat(201),'nul\0']) expect(()=>searchPath(q)).toThrow();
  expect(()=>searchPath('text','toString' as never)).toThrow();
});
it('requires fixed formal version evidence for ready file hits',()=>{
  expect(checkedSearch({items:[hit()],next_cursor:null}).items.length).toBe(1);
  for(const h of [{...hit(),resource:{type:'file',id:file}},{...hit(),indexed_at:null},{...hit(),text_state:'invented'},{...hit(),match_types:[]},{...hit(),resource:{type:'toString',id:file}}]) expect(()=>checkedSearch({items:[h],next_cursor:null})).toThrow();
});
it('rejects duplicate results and oversized or invalid snippets',()=>{
  for(const items of [[hit(),hit()],[{...hit(),snippet:'x'.repeat(241)}],[{...hit(),project_id:'guess'}],[{...hit(),updated_at:'bad'}]]) expect(()=>checkedSearch({items,next_cursor:null})).toThrow();
});
it('pins preview to an exact version and Unicode character offset',()=>{
  expect(fileTextPath(file,version,4000)).toBe('/files/'+file+'/versions/'+version+'/text?limit=4000&offset=4000');
  expect(checkedFileText(text(),file,version,0).next_offset).toBe(5);
  expect(()=>checkedFileText(text(),file,file,0)).toThrow(); expect(()=>fileTextPath(file,version,-1)).toThrow();
});
it('rejects mismatched chunk boundaries, hashes and forged failure plaintext',()=>{
  for(const v of [{...text(),next_offset:6},{...text(),next_offset:0},{...text(),total_characters:4},{...text(),source_sha256:'bad'},{...text(),state:'failed'},{...text(),offset:1},{...text(),extractor:'other'},{...text(),text:'x'.repeat(4001)}]) expect(()=>checkedFileText(v,file,version,0)).toThrow();
});
it('accepts explicit pending and unsupported states without derived body',()=>{
  for(const state of ['pending','unsupported','blocked','failed'] as const) {
    const v={...text(),state,text:'',text_sha256:null,total_characters:null,next_offset:null,error_code:state==='pending'?null:'FILE_TEXT_FORMAT_UNSUPPORTED',indexed_at:state==='pending'?null:time};
    expect(checkedFileText(v,file,version,0).state).toBe(state);
  }
});
