# 秘书文本更新、隔离草稿与正式版本验证

日期：2026-10-25。对应[增量52](../../../development/52-secretary-file-updates.md)和 GAP-04／11／12。验证使用原生 Windows Go／PostgreSQL、真实 Web／Taro H5 页面及受控对象字节，没有使用 Docker、WSL、虚拟机、模拟器或子代理。

## 1. 当前结果

| 证据 | 结果 | 证明范围 |
| --- | --- | --- |
| [完整后端固定来源回归](backend-result.json) | 14批、110个顶层测试、658条结果、596个叶场景通过，0失败／跳过 | 全部自动发现的测试均执行，不混用旧来源部分批次 |
| [Web测试与构建](web-build.json) | 34项、类型及生产构建通过 | React／Vite和共享契约，不是生产部署 |
| [移动测试与编译](mobile-build.json) | 58项、类型、微信／抖音编译通过 | Taro公共实现；没有运行两个小程序平台或Android真机 |
| [Runtime与Skill检查](runtime-build.json) | 23项和8个草稿包校验通过 | 受控SDK工厂、HTTP／MCP、pending与真实提交凭证；没有调用真实模型 |
| [Web实际操作](../secretary-file-updates-web/result.json) | 6条通过 | 实际服务端差异、正式版本、第二副本排队、明确范围和取消 |
| [Taro H5实际操作](../secretary-file-updates-mobile/result.json) | 6条通过 | 同上；真实MFA、隔离原生API及PostgreSQL，不等于原生端验收 |
| [Linux amd64](linux-build.json) | 仅编译通过 | 没有执行或部署该产物 |
| [开发环境](development-file-updates.json) | 51项迁移、13项文件更新守卫、五张证据表只追加、10项基础探测及未认证文件入口拒绝通过 | 活动运行配置0、8个草稿包、专用云读写默认关闭，未部署生产 |
| [最终来源与清理](final-source-check.json) | 15项全部通过 | 构建、两组页面和当前来源一致；隔离库0、私有夹具不存在、5189释放、原开发入口就绪 |

完整后端于2026-10-25 09:39（Asia/Taipei）完成，摘要为`f798ad627b4cab1d568a9e3802f16283af14eb36b6c398e0c13d6f81f5fb525f`；[14个批次](backend-batch-progress.json)均保持该来源。两组浏览器无页面脚本／服务错误，所属浏览器和上下文已关闭、隔离数据库清理通过。5189临时服务由各自启动器停止，原5178及更新后API 18080就绪；最终来源复算全部通过。

## 2. 关键业务验证

`TestAgentFileUpdates` 通过真实HTTP／MCP、原生PostgreSQL及既有文件流程，验证明确基础版本与元数据修订、准确五项工具、实际BOM／CRLF字节、Unicode编辑与真实差异、隔离分片、完整摘要、扫描状态、正式候选和最终提交。对象提供商、扫描判定和原生隔离证明使用受控本地实现，没有运行真实云、杀毒引擎或模型进程。

未扫描或候选未就绪返回pending，不能输出业务已完成；准确计划摘要通过后，正式版本、当前文件头、副本记录、审计／Outbox、业务操作与工具回执原子提交。注入回执失败时整笔回滚；同键重放不重复上传，同一运行重复正式提交不增加版本，第二副本仍明确为queued。GetRun和事件历史均保留真实更新视图。

普通上传接口不能读取或提交秘书草稿；普通队列在LIMIT前过滤agent渠道，专用limit=1场景验证人工任务不会被隐藏草稿挤出。网络写入钩子中真实取消、并发人工提交文件头、变更组织授权，均阻止后续落证或提交；失败尝试Abort。1秒数据库预算证明远程写入期间没有占用授权锁。

文本编辑单元场景覆盖BOM／CRLF、中文／emoji、1MiB边界、无变化、空结果、混合换行、裸CR、NUL、无效编码、越界、重叠和过大修改。专用草稿身份拒绝普通云角色兜底；原生执行进程环境测试确认新增写入凭据被剥离。

## 3. 尝试记录

[历史基线](attempts/prior-baseline/)与以下尝试独立保留，均不作为当前完整通过结果：

- [事件历史和类型修复前](attempts/before-event-log-and-type-fixes/reason.json)：真实页面发现事件日志没有加载文件更新视图，构建发现新增测试夹具类型错误；修复后重新验证。
- [输出引用Schema修复前](attempts/before-output-ref-schema/reason.json)：新增pending草稿运行时用例发现内联引用枚举遗漏；补齐所有对应位置后23项检查通过。
- [移动来源清单修复前](attempts/before-mobile-manifest-fix/reason.json)：6条页面操作通过，但构建和浏览器清单不一致，因此仍判失败；统一清单后重新编译及操作通过。
- [普通上传队列过滤前](attempts/before-upload-queue-filter/reason.json)：发现分页先取秘书草稿可能产生人工空页；修正SQL并增加真实limit=1场景，旧来源部分批次及构建／页面证据不计入最终回归。

## 4. 验收限制

本增量只支持1字节至1MiB的UTF-8文本／Markdown现行文件更新。CSV／JSON仍为只读；PDF／Office／二进制、创建文件、移动目录、批量与跨运行草稿恢复尚未完成。运行已回复pending、结束、取消或过期后不会自动继续提交，员工须核对现行版本后明确发起新请求。

正常失败尝试终止分片，不能证明崩溃／未知云回执后的物理对象已经清除。应用侧物理清理执行器仍未接通，真实临时对象／分片生命周期规则需部署验收。正式主副本已验证与第二副本已完成校验是不同事实。

本轮不证明真实OSS／R2、真实模型与原生隔离发布、Skill发布、TG、Android与小程序真机、120GB／2TB、生产部署或恢复。八个实际Skill仍为draft；上述工作继续按[28项Goal差异](../../../delivery/03-goal-gap-assessment.md)推进。
