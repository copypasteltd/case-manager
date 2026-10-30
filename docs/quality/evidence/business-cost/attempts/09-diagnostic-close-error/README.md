# Diagnostic teardown error

This current-input native bundle attempt failed during an owned-process diagnostic. The read-only inspection returned its process/IPC handle types and no active native requests. The diagnostic teardown then incorrectly called CommonJS `require` in Metro's ESM global context, causing an uncaught `ReferenceError` and terminating the owned build. This was an error in the temporary diagnostic, not an application test or a successful compilation.

The nonzero build record remains failed. The raw log is preserved outside Git at `app/.local/business125-native-diagnostic-close.log`. The temporary diagnostic now uses Node22's `process.getBuiltinModule` to close its own inspector. No source, business input or acceptance condition is changed by that correction. The same final-input bundle is compiled again; the other planned checks continue against their unchanged final input digest.
