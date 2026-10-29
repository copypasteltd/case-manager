"""Run the existing Go cases unchanged against the owned durable native PG cluster."""
import subprocess
import sys
from notification_retry_native_cluster import APP, test_environment

arguments = sys.argv[1:]
package = './...'
if '--package' in arguments:
    index = arguments.index('--package')
    assert index + 1 < len(arguments) and arguments[index + 1] in ('./...','./internal/httpapi')
    package = arguments[index + 1]
    arguments = arguments[:index] + arguments[index+2:]
raise SystemExit(subprocess.call([str(APP/'.tools/go/bin/go.exe'),'test',package,'-count=1',*arguments],
    cwd=APP/'backend',env=test_environment()))
