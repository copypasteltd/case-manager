"""Fetch a pinned native Go toolchain, verifying the official SHA-256 before use.

No administrator install, PATH mutation, virtualization, or certificate bypass.
The mirror distributes the official archive; its bytes must match go.dev.
"""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import subprocess
import zipfile

APP=Path(__file__).resolve().parents[1]
TOOLS=APP/'.tools'
VERSION='1.27.3'
SIZE=79010309
SHA256='f26081f438319757f8a1b46d7d0b2c5ec7583d4038e485e0aa725cfde7885fff'
URL='https://mirrors.aliyun.com/golang/go'+VERSION+'.windows-amd64.zip'
PART_SIZE=1024*1024

def fetch(index):
    start=index*PART_SIZE
    end=min(start+PART_SIZE,SIZE)-1
    part=TOOLS/'downloads'/'go-parts'/str(index)
    if part.exists() and part.stat().st_size==end-start+1:
        return part
    args=['curl.exe','--fail','--ssl-revoke-best-effort','--silent','--show-error',
          '--connect-timeout','12','--max-time','120','--retry','1','--retry-delay','1',
          '--range',str(start)+'-'+str(end),URL,'-o',str(part)]
    subprocess.run(args,check=True)
    if part.stat().st_size!=end-start+1:
        raise ValueError('Range response length differs for part '+str(index))
    print(json.dumps({'go_part':index,'bytes':part.stat().st_size}),flush=True)
    return part

def main():
    (TOOLS/'downloads'/'go-parts').mkdir(parents=True,exist_ok=True)
    count=(SIZE+PART_SIZE-1)//PART_SIZE
    with concurrent.futures.ThreadPoolExecutor(max_workers=16) as pool:
        list(pool.map(fetch,range(count)))
    archive=TOOLS/'downloads'/('go'+VERSION+'.windows-amd64.zip')
    digest=hashlib.sha256()
    with archive.open('wb') as out:
        for index in range(count):
            data=(TOOLS/'downloads'/'go-parts'/str(index)).read_bytes()
            digest.update(data)
            out.write(data)
    if digest.hexdigest()!=SHA256:
        raise ValueError('Official Go archive checksum mismatch; refusing extraction')
    with zipfile.ZipFile(archive) as source:
        for entry in source.infolist():
            target=(TOOLS/entry.filename).resolve()
            if not target.is_relative_to(TOOLS.resolve()):
                raise ValueError('Unsafe archive path')
        source.extractall(TOOLS)
    subprocess.run([str(TOOLS/'go'/'bin'/'go.exe'),'version'],check=True)
    print(json.dumps({'go':VERSION,'sha256':SHA256,'official_checksum_verified':True}))

if __name__=='__main__':
    main()
