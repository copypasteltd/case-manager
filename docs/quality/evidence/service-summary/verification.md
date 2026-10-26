# 增量62：服务事件摘要验证记录

本轮功能实现及验证已完成；完整公司平台 Goal 仍在推进。全部测试在原生进程及所属 PostgreSQL 上执行，没有启动 Docker、WSL、虚拟机或模拟器。

| 验证 | 实际结果 | 证据 |
| --- | --- | --- |
| 完整后端 | 7 批、135 个顶层测试、800 条结果、723 个末级场景；无失败、跳过或取消 | [原生结果](result.json) |
| Web | 54 项测试及正式构建通过 | [构建](web-build.json) |
| Web 浏览器 | 3 项通过；实际消息打开固定证据、旧故障与新健康观测分离、服务选择与共同权限读取 | [浏览器](../service-summary-web/result.json) |
| 移动源码 | 62 项、类型检查、微信及抖音编译通过；13 个公共页面 | [构建](mobile-build.json) |
| Taro 浏览器 | 2 项通过；实际历史快照与服务资料选择 | [浏览器](../service-summary-mobile/result.json) |
| 运行时 | 32 项及八个 draft 源码包校验通过 | [运行时](runtime-build.json) |
| Linux | linux/amd64 编译通过，未执行产物、未部署 | [编译](linux-build.json) |
| 所属开发环境 | 61 项迁移；三个快照约束、只读应用权限核对通过；执行配置 0、测试库 0 | [环境](development-service-summary.json) |
| 最终来源 | 22 项通过，构建及浏览器均对应当前来源 | [核对](final-source-check.json) |

后端来源：`e458981fefd528186c3e2edfa41e0d75622c6af4a0d4195ffe2af21d3ce21085`。验收报告保留来源文件清单及摘要，独立批次只在代码未变化时复用；没有将未完成进程算作通过。

## 已验证的业务边界

- 准确服务选择与项目／公司范围、会话成员共同权限、AI 分类政策。
- 签名观测驱动真实状态；维护覆盖保留原始故障；跨服务事件逐项授权。
- 时间窗成对、不得晚于服务器读取时刻、最多31天；只读技能拒绝写操作。
- 快照与工具回执在同一事务绑定；数据库拒绝修改、删除与无回执伪造。
- 新的健康观测不改写旧证据，工具回放和历史来源可独立读取；当前政策撤回仍会阻止回看。
- TG `/incident` 实际创建任务、领取执行凭据、调用只读服务工具并保存有固定来源的门户结果。

Taro 验收期间修正了测试脚本的按钮可访问名称及新增验收模式的来源清单分支，补上实际消息响应和来源可见性的断言，并在最终来源上重新执行。旧失败截图保留用于定位；通过结论来自最后一次完整验收，未通过降低断言或忽略服务错误获得。

## 清理与实际边界

两套浏览器共关闭 2 个所属上下文和全部所属浏览器。临时 Vite／H5 审查服务、隔离 HTTP 服务与数据库均已释放；私密 fixture 文件已删除，5189 端口空闲。保留的开发 API、通知 worker 与原有 Vite 已核对进程身份。

没有调用真实模型、TG、OSS 或 R2，也没有生产激活。源码仍为八个 draft，事件摘要1.1.0；真实隔离证明、模型评测、监控采集器、TG 群披露／外发、真实设备及120GB传输另行验收。既有 Windows 短连接未认证 POST 重置现象不由本增量宣称修复，环境探针沿用鉴权先于解码的空请求，真实资料功能由原生数据库及浏览器用例验证。

运行命令：`python app/scripts/verify_backend_batches.py --evidence-group service-summary --batch-size 20`、`verify_web.py service-summary`、`verify_mobile.py service-summary`、`verify_agent_runtime.py service-summary`、`verify_linux.py service-summary`、`run_agent_browser.py --service-summary`、`run_mobile_browser.py --service-summary`、`verify_service_summary_development.py`、`verify_service_summary_sources.py`。
