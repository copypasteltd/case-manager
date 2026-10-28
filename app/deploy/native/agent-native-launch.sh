#!/bin/sh
# Fixed executable/socket; caller arguments are checked again by the root broker.
exec /opt/copypaste/bin/agent-native launch --socket /var/lib/copypaste/agent-native/control.sock "$@"
