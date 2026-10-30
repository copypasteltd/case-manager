# Native bundle timeout / 原生JS编译超时

The first current125 Android JS compilation exceeded the published900-second process timeout. The failure record is preserved here; the original native log is retained outside Git at `app/.local/business125-native-timeout.log`. Metro reported visiting61,986 added/modified files; this observation does not establish the cause of the timeout or successful compilation. The owned wrapper and Node process tree exited, and the known owned process IDs were checked before retrying.

The retry uses the identical current source, command, four-worker Metro adapter, generated-output exclusions, native Windows filesystem bridge and no-Expo map guard. No application input, source-map guard or acceptance requirement is bypassed. Current successful evidence, if produced, is recorded separately at the evidence root; this failed attempt remains failed. Neither attempt creates or signs an APK.

首轮超过原900秒边界，保留失败与原日志；不能用文件发现日志推断已编译成功。确认自有进程退出后，以相同输入与命令重试。无APK、签名或正式Android验收声明。
