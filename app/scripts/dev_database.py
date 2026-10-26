"""Start/stop this repository's native local PostgreSQL fixture cluster.

The production application never starts a database or uses this directory.
Credentials are generated, ignored by Git, and never printed.
"""
import argparse
import base64
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
from urllib.parse import quote
from windows_native_path import native_app, release

APP=native_app()
LOCAL=APP/'.local'
BIN=APP/'.tools'/'pgsql'/'bin'
DATA=LOCAL/'postgres'
PORT=55432

def run(name,args,**kwargs):
    flags=subprocess.CREATE_NO_WINDOW if os.name=='nt' else 0
    # A Windows postmaster inherits pg_ctl handles. Files let pg_ctl return
    # without waiting for the server to close inherited pipe descriptors.
    with tempfile.TemporaryFile() as output, tempfile.TemporaryFile() as errors:
        result=subprocess.run([str(BIN/(name+'.exe')), *args],creationflags=flags,
                              stdout=output,stderr=errors,**kwargs)
        output.seek(0); errors.seek(0)
        result.stdout=output.read(); result.stderr=errors.read()
    if result.returncode:
        raise subprocess.CalledProcessError(result.returncode,result.args,result.stdout,result.stderr)
    return result

def load_env():
    return json.loads((LOCAL/'environment.json').read_text(encoding='utf-8'))

def wait_ready(port, timeout=180):
    """A live postmaster can spend more than 30 seconds recovering after interruption.

    Poll that same owned cluster; an observation timeout must never restart it.
    Each native readiness probe is bounded and output contains no credentials.
    """
    deadline=time.monotonic()+timeout
    progress_at=time.monotonic()+15
    while time.monotonic()<deadline:
        try:
            run('pg_ctl',['-D',str(DATA),'status'])
            run('pg_isready',['-h','127.0.0.1','-p',str(port),'-t','1'])
            return
        except subprocess.CalledProcessError:
            if time.monotonic()>=progress_at:
                print('Waiting for owned development PostgreSQL recovery/readiness; no restart requested',flush=True)
                progress_at=time.monotonic()+30
            time.sleep(1)
    raise RuntimeError('Owned PostgreSQL did not become ready; inspect its state and recovery log before any restart')

def main():
    global PORT
    parser=argparse.ArgumentParser()
    parser.add_argument('action',choices=['start','stop','status'])
    args=parser.parse_args()
    LOCAL.mkdir(exist_ok=True)
    if args.action=='stop':
        if not DATA.exists():
            print('No local development cluster exists');return
        try:
            run('pg_ctl',['-D',str(DATA),'-m','fast','-w','-t','30','stop'])
        except subprocess.CalledProcessError as e:
            if not (DATA/'postmaster.pid').exists(): release();print('Local development cluster is stopped');return
            raise RuntimeError('Unable to stop owned development PostgreSQL') from e
        release();print('Local development PostgreSQL stopped');return
    if args.action=='status':
        try:
            run('pg_ctl',['-D',str(DATA),'status'])
            port=load_env().get('_postgres_port',PORT)
            try:
                run('pg_isready',['-h','127.0.0.1','-p',str(port),'-t','1'])
                print('Local development PostgreSQL is running and ready')
            except subprocess.CalledProcessError:
                print('Local development PostgreSQL is running but not yet ready; recovery may be in progress')
        except subprocess.CalledProcessError: print('Local development PostgreSQL is stopped')
        return
    envfile=LOCAL/'environment.json'
    if not envfile.exists():
        superuser=secrets.token_hex(24);application=secrets.token_hex(24);migration=secrets.token_hex(24)
        env={
            'CP_ENV':'development','CP_HTTP_ADDR':'127.0.0.1:8080','CP_PUBLIC_ORIGIN':'http://localhost:5173',
            'CP_ALLOWED_ORIGINS':'http://localhost:5173,http://127.0.0.1:5173,http://127.0.0.1:8080',
            'CP_DATABASE_URL':'postgres://copypaste_app:'+quote(application)+'@127.0.0.1:'+str(PORT)+'/copypaste?sslmode=disable',
            'CP_MIGRATION_DATABASE_URL':'postgres://copypaste_migrator:'+quote(migration)+'@127.0.0.1:'+str(PORT)+'/copypaste?sslmode=disable',
            'CP_CURSOR_KEY':secrets.token_hex(32),'CP_MFA_KEY':base64.b64encode(secrets.token_bytes(32)).decode(),
            'CP_STORAGE_DRIVER':'disabled',
            '_superuser_password':superuser,'_application_password':application,'_migration_password':migration}
        envfile.write_text(json.dumps(env,indent=2)+'\n',encoding='utf-8')
    else:
        env=load_env()
    if '_postgres_port' not in env:
        with socket.socket() as reservation:
            reservation.bind(('127.0.0.1',0))
            selected=reservation.getsockname()[1]
        env['_postgres_port']=selected
        for key in ('CP_DATABASE_URL','CP_MIGRATION_DATABASE_URL'):
            env[key]=env[key].replace(':55432/',':'+str(selected)+'/')
        env['CP_HTTP_ADDR']='127.0.0.1:18080'
        env['CP_ALLOWED_ORIGINS']+=',http://localhost:5178,http://127.0.0.1:5178,http://127.0.0.1:18080'
        env['CP_PUBLIC_ORIGIN']='http://localhost:5178'
        envfile.write_text(json.dumps(env,indent=2)+'\n',encoding='utf-8')
    PORT=int(env['_postgres_port'])
    if not (DATA/'PG_VERSION').exists():
        passwordfile=LOCAL/'postgres-password.tmp'
        passwordfile.write_text(env['_superuser_password'],encoding='ascii')
        try:
            run('initdb',['-D',str(DATA),'-U','postgres','-A','scram-sha-256','--pwfile='+str(passwordfile),'--encoding=UTF8','--locale=C'])
        except subprocess.CalledProcessError as e:
            raise RuntimeError('Native initdb failed: '+e.stderr.decode(errors='replace')) from e
        finally:
            passwordfile.unlink(missing_ok=True)
    try: run('pg_ctl',['-D',str(DATA),'status'])
    except subprocess.CalledProcessError:
        run('pg_ctl',['-D',str(DATA),'-l',str(LOCAL/'postgres.log'),'-o','-h 127.0.0.1 -p '+str(PORT),'-W','start'])
    wait_ready(PORT)
    sql="""
    SELECT 'CREATE ROLE copypaste_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD ''%s'''
      WHERE NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_migrator') \\gexec
    SELECT 'CREATE ROLE copypaste_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD ''%s'''
      WHERE NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='copypaste_app') \\gexec
    SELECT 'CREATE DATABASE copypaste OWNER copypaste_migrator'
      WHERE NOT EXISTS(SELECT 1 FROM pg_database WHERE datname='copypaste') \\gexec
    """%(env['_migration_password'],env['_application_password'])
    process_env={**os.environ,'PGPASSWORD':env['_superuser_password']}
    run('psql',['-h','127.0.0.1','-p',str(PORT),'-U','postgres','-d','postgres','--no-psqlrc','--quiet','--set=ON_ERROR_STOP=1'],input=sql.encode(),env=process_env)
    print(json.dumps({'native_postgresql':'18.6','host':'127.0.0.1','port':PORT,'state':'running','credentials_printed':False}))

if __name__=='__main__':
    main()
