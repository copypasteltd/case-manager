# Native test-output file lock / 原生测试输出文件锁

The first complete build produced the business bundle, then Android Gradle could not delete its test-output binary while a separately owned Java preflight was accessing that output. The preflight subsequently passed. This complete-build attempt is retained as failed; no APK acceptance is inferred. The identical frozen complete-build command is retried alone after the preflight ended. No application, plan or native-build helper bytes changed.

完整构建在 Android 单元测试输出文件锁处失败；独立 Java 预检查随后通过。本次失败原样保留，不据此认定 APK 通过。预检查结束后单独重试相同冻结构建命令。
