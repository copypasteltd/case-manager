"""Provide an ASCII DOS path alias for native PostgreSQL on Windows.

This aliases existing files; it does not create a machine, subsystem or emulator.
The database bytes remain under app/.local. Only an owned exact mapping is removed.
"""
import ctypes
import json
import os
from pathlib import Path

APP=Path(__file__).resolve().parents[1]
RECORD=APP/'.local'/'postgres-path-alias.json'

def query(drive):
    buffer=ctypes.create_unicode_buffer(32768)
    if ctypes.windll.kernel32.QueryDosDeviceW(drive,buffer,len(buffer)):
        return buffer.value
    return None

def native_app():
    if os.name!='nt' or str(APP).isascii():
        return APP
    APP.joinpath('.local').mkdir(exist_ok=True)
    target='\\??\\'+str(APP)
    if RECORD.exists():
        info=json.loads(RECORD.read_text(encoding='utf-8'))
        actual=query(info['drive'])
        if actual==target:
            return Path(info['drive']+'\\')
        if actual is not None:
            raise RuntimeError('Recorded native path alias is now owned by another target')
        if not ctypes.windll.kernel32.DefineDosDeviceW(1|8,info['drive'],target):
            raise ctypes.WinError()
        return Path(info['drive']+'\\')
    for letter in 'RSTUVWXYZ':
        drive=letter+':'
        if query(drive) is not None:
            continue
        if not ctypes.windll.kernel32.DefineDosDeviceW(1|8,drive,target):
            raise ctypes.WinError()
        RECORD.write_text(json.dumps({'drive':drive,'target':target},ensure_ascii=False)+'\n',encoding='utf-8')
        return Path(drive+'\\')
    raise RuntimeError('No unused DOS path alias is available')

def release():
    if os.name!='nt' or not RECORD.exists():
        return
    info=json.loads(RECORD.read_text(encoding='utf-8'))
    if query(info['drive'])==info['target']:
        if not ctypes.windll.kernel32.DefineDosDeviceW(1|2|4|8,info['drive'],info['target']):
            raise ctypes.WinError()
    RECORD.unlink()
