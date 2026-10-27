"""Preserve published93 bytes and reconstruct its complete capture from Git."""
import ast
import hashlib
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[2]
APP=ROOT/'app'
REF='54f340c1c17c59bc70ac640330a26590c5adad2f'
E=ROOT/'docs/quality/evidence/mobile-files'
def names(paths):
    return sorted(x for x in subprocess.check_output(['git','ls-tree','-r','--name-only',REF,'--',*paths],cwd=ROOT).decode().splitlines() if x)
def objects(paths):
    raw=subprocess.check_output(['git','cat-file','--batch'],input=('\n'.join(REF+':'+p for p in paths)+'\n').encode(),cwd=ROOT)
    out={};at=0
    for p in paths:
        end=raw.index(b'\n',at);header=raw[at:end].split();assert header[1]==b'blob';size=int(header[2]);at=end+1;out[p]=raw[at:at+size];at+=size+1
    assert at==len(raw);return out
def digest(paths,data):
    h=hashlib.sha256()
    for p in paths:h.update(p.encode()+b'\0'+data['app/'+p]+b'\0')
    return h.hexdigest()
def reconstruct():
    source=objects(['app/scripts/agent_indexed_search_sources.py'])['app/scripts/agent_indexed_search_sources.py']
    tree=ast.parse(source);scopes=next(ast.literal_eval(n.value) for n in ast.walk(tree) if isinstance(n,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='scopes' for t in n.targets))
    complete=[p.removeprefix('app/') for p in names(['app/'+s for s in scopes])]
    clients={};all_paths=set(complete);inputs={}
    for client in ('web','mobile'):
        base='docs/quality/evidence/agent-indexed-search-'+client+'/'
        raw=objects([base+'result.json',base+'source-manifest.json'])
        result=json.loads(raw[base+'result.json']);manifest=json.loads(raw[base+'source-manifest.json'])
        all_paths.update(manifest['files']);inputs[client]=(result,manifest)
    data=objects(['app/'+p for p in sorted(all_paths)]);full=digest(complete,data)
    for client,(result,manifest) in inputs.items():
        sha=digest(manifest['files'],data)
        assert result['passed'] and full==result['build_source_sha256']==result['build_source_after_sha256']
        assert sha==manifest['sha256']==result['source_sha256']==result['source_after_sha256']
        clients[client]={'source_count':len(manifest['files']),'source_sha256':sha,'reconstructed':True}
    retained=names(['docs/quality/evidence/agent-indexed-search','docs/quality/evidence/agent-indexed-search-web','docs/quality/evidence/agent-indexed-search-mobile'])
    media=[p for p in names(['docs/media']) if Path(p).name.startswith('agent-indexed-search-') and Path(p).suffix in ('.png','.mp4','.gif')]
    old=objects(retained+media);records=[]
    for p in retained+media:
        assert (ROOT/p).read_bytes()==old[p],'Published history changed: '+p
        records.append({'path':p,'sha256':hashlib.sha256(old[p]).hexdigest(),'bytes':len(old[p])})
    assert len(media)==8
    return {'passed':True,'commit':REF,'complete_startup_source':{'source_count':len(complete),'sha256':full,'reconstructed':True},'clients':clients,'preserved':records,'preserved_media_count':len(media),'scope':'Exact published93 Git objects; original status and bytes retained, no current-code or external acceptance claim.'}
if __name__=='__main__':
    E.mkdir(parents=True,exist_ok=True);v=reconstruct();(E/'historical-93.json').write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf-8');print(json.dumps({'passed':True,'preserved':len(v['preserved']),'media':v['preserved_media_count']}))
