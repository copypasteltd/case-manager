# Attempt03: local check timeouts / 本地检查超时

The unchanged final source produced 350/352 Web passes; the existing finance-FX query and project-schedule instant tests exceeded their unchanged 5000ms deadlines. This attempt is failed, not current acceptance. Both test names and failures remain in the retained private Vitest report and log, with hashes in the source-bound record. No assertions or deadlines were relaxed.

The second Web browser attempt never reached browser execution: Vite reported ready after 203129ms and the owned-server readiness check had already failed. The result retains its failure, unchanged build-source hash, and successful fixture/server/browser-runner/database cleanup. No new screenshot or recording was produced. The earlier successful Web capture stays separately in attempt02, not current acceptance.

Final commands and the current Web capture are rerun unchanged, with independent native resources closed between checks. The first combined Android timeout and the later Web unknown-outcome repair are separately disclosed in attempt02.

源码未变化，本次Web执行350／352通过；原有汇率查询与项目时间测试超过原5000ms限制，未修改断言或超时。第二次Web页面检查在Vite启动阶段失败，没有浏览器结果或新媒体；自有资源和临时数据库均已清理。本次失败与此前成功截图均保留，均不冒充最终通过。

The same selected backend run recorded139 passing tests/subtests and failed only the TestBusinessCostRecording parent: its owned database DROP cleanup exceeded the unchanged independent two-minute limit at server_integration_test.go:86. Application assertions were not relaxed; the combined run remains failed and is rerun. Backend fixture cleanup and the final zero-disposable-database inspection remain required.

同次后端139条记录通过，TestBusinessCostRecording父项因隔离库DROP清理超过原两分钟限制失败；应用业务断言不因此放宽。最终仍需原命令重跑和零遗留检查库核对。
