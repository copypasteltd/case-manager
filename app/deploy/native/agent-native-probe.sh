#!/bin/sh
# No key, environment override or arbitrary signing mode is exposed to the worker.
exec /opt/copypaste/bin/agent-native probe --socket /var/lib/copypaste/agent-native/control.sock
