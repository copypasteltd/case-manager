"""Create a local SHA-pinned profile from an already installed native tool tree.

Does not download, install, activate, execute tools, start containers or write Git.
The profile and native dependencies must remain outside tracked files.
"""
import argparse
import hashlib
import json
from pathlib import Path

def build(root, temp, tessdata):
    root = root.resolve(strict=True)
    if str(root).startswith('\\\\') or not temp.is_absolute():
        raise ValueError('Local absolute root/temp paths required.')
    def artifact(file):
        file = root / file
        if file.is_symlink() or file.resolve(strict=True) != file.absolute() or not file.is_file() or not 0 < file.stat().st_size <= 128 << 20:
            raise ValueError('Regular bounded pinned files required: ' + str(file))
        return {'path':file.relative_to(root).as_posix(),'sha256':hashlib.sha256(file.read_bytes()).hexdigest()}
    tools = {}
    for name in ('pdfinfo','pdftotext','pdftoppm','tesseract'):
        found = [p for p in (root/'Library/bin'/(name+'.exe'),root/'bin'/name) if p.is_file()]
        if len(found) != 1: raise ValueError('One explicitly installed tool required: '+name)
        tools[name] = artifact(found[0])
    data = root / tessdata
    if not data.resolve(strict=True).is_relative_to(root): raise ValueError('Models must be under the tool root.')
    assets = {data/(lang+'.traineddata') for lang in ('chi_tra','chi_sim','eng')}
    for base in (root/'Library/bin', root/'lib'):
        if base.is_dir():
            assets.update(p for p in base.iterdir() if p.is_file() and (p.suffix.lower()=='.dll' or '.so' in p.name))
    for base in (root/'share/poppler',root/'Library/etc/fonts',root/'etc/fonts'):
        if base.is_dir(): assets.update(p for p in base.rglob('*') if p.is_file())
    result={'version':1,'root':str(root),'temp_root':str(temp.absolute()),'tessdata':data.relative_to(root).as_posix(),'tools':tools,'assets':[artifact(p) for p in sorted(assets)]}
    if len(result['assets'])>4096: raise ValueError('Too many pinned assets.')
    return result

if __name__ == '__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--root',type=Path,required=True);p.add_argument('--temp',type=Path,required=True);p.add_argument('--tessdata',required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args()
    profile=build(a.root,a.temp,a.tessdata)
    a.output.parent.mkdir(parents=True,exist_ok=True)
    with a.output.open('x',encoding='utf-8',newline='\n') as f: f.write(json.dumps(profile,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps({'profile_created':True,'tools':len(profile['tools']),'assets':len(profile['assets']),'not_a_process_sandbox':True}))
