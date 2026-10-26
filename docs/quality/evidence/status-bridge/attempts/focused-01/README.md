初次目标验证失败，保留原始日志／报告。Windows追加句柄不能直接截断，现改为持有独占锁的读写句柄并在追加前定位文件末尾；来源发布接口原本返回200，修正测试的201断言。最终验证另存，不改写本次结果。

Initial focused run failed. Windows append handles cannot truncate a torn journal tail; use an exclusively owned read/write handle and seek to EOF before each durable append. The original source-publication endpoint returns 200; correct the test expectation. Raw evidence remains unchanged.
