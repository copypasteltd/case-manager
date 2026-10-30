# Preserved native startup attempt

The unchanged published native builder exited1 with unknown command bundle before compiling a new bundle. Its declared owned TEMP/TMP directory had not yet been created. Preserve the original result and lossless log, create that empty ignored native cache directory, then rerun the same command/profile/source. This result is not a native build pass.
