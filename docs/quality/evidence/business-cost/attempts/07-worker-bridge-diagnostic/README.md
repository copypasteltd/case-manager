# Explicitly stopped diagnostic / 主动结束的诊断

The unchanged-command retry reached file-map processing but was explicitly stopped after ownership checks; its nonzero result remains failed, not a timeout or a pass. The native process and its verified owned children were terminated together. The raw log is retained outside Git at `app/.local/business125-native-worker-diagnostic.log`.

Inspection of the installed Metro file-map source showed `forkOptions.execArgv=[]`; its actual worker command lines had no parent `--require` bridge. The worker uses an asynchronous native `readlink`. These facts establish that workers did not inherit the parent's disclosed Windows filesystem bridge; they do not independently prove the full cause of every delay.

The current bundle-only builder now sets a fixed, checkout-owned `NODE_OPTIONS` require for the same bridge, for its own process tree only. Arbitrary inherited Node options are still excluded. The adapter writes the bridge before starting Node; native file contents and link targets are used unchanged. Four workers, the source-map no-Expo guard and no-APK scope remain. The current input digest therefore changed: all nine planned command records are executed again; earlier passed application checks are retained separately in attempt08. Browser inputs do not consume this bundle builder and retain their independently verified source binding.
