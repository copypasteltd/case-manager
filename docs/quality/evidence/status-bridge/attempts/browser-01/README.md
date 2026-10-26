两次实际浏览器检查均因检查器将数值投影序号 2 按字符串比较而失败；原生 CLI 的签名写入和进程重启去重已成功。首次报告未在重跑前冻结，本目录准确保留第二次完整报告、来源清单和检查脚本，不把这两次运行计为通过。两次 fixture、所属上下文和临时服务均正常清理。后续修正仅限检查器类型断言。

Two actual browser runs failed at the same checker assertion comparing numeric projection sequence 2 with a string. Native CLI delivery and restart deduplication succeeded. The first report was not frozen before rerun; this directory retains the exact second report, source list and checker, and neither run counts as passed. Both owned fixtures, contexts and temporary services were released. The correction changes only the checker assertion.
