# Native release evidence / 原生发布工程证据

[verification.json](verification.json)记录本轮准确工程源码、40项新测试、真实构建归档、工作流解析和原118来源复用；[build.json](build.json)记录784文件、三个ELF、Web／运行器及91项结构清单。源码、归档、编译日志、测试日志和秘密资料在私有目录，未提交Git。

[preliminary-attempts.json](preliminary-attempts.json)保留初期环境限制及两次打包问题的来源摘要；各早期执行只保留原来源与范围，不计最终通过；新增四项中断／完整性问题的实际失败重现及修复见[journal-interruption-attempt.json](journal-interruption-attempt.json)。Windows指针及进程效果为明确测试替身，没有执行Linux服务、真实CI、HTTPS或生产PG。

本轮不重跑118完整业务回归、不重新采集媒体、不重新查询依赖公告；原316媒体和原118证据保持Git字节。完整边界见[验证说明](../../119-native-release-verification.md)。
