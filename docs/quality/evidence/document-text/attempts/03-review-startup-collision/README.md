# Preserved concurrent review startup

Both captures were started before the other owned preparation had completed. The fixed fixture file/ports collided: one browser action ran but its own native fixture exited1; the other browser stopped before login. Both final runner results correctly failed. All owned browsers, contexts, proxy/server, native fixtures and disposable databases closed. No failed runner is acceptance. The same unchanged application and capture sources are rerun strictly sequentially. Earlier screenshot bytes are retained under original-media/.
