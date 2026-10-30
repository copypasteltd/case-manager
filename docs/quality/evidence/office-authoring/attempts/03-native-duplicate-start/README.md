# Interrupted development build attempt

Two owned check invocations started the same native bundle target concurrently before a completion record existed. Both owned process trees were stopped; unrelated services were preserved. No pass or current artifact is attributed to this attempt. The final native command is executed once after the overlap, with a new build and current source-bound record. Partial logs remain ignored because native local diagnostics contain machine paths.
