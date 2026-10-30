# Increment123 · DOCX 功能检查 / Executed verification

本轮以有界 DOCX 提取和四端共用消费链路为范围。所有当前通过记录绑定最终源码；旧发布记录按原字节保存，旧回归或 APK 不改标为当前执行。[实现及使用](../development/123-docx-text-extraction.md)与[当前范围](../delivery/04-current-scope-and-acceptance.md)继续约束发布描述。

## 本轮实现与实际检查

| 编号 | 功能／检查 | 当前结果与证据 |
|---|---|---|
| D123-01 | Word 关系定位和格式解析 | 独立 `docx-v2` 读取准确包主部件；主体、表格、控件、文本框、修订与字段标记；Referenced stories、Strict 命名空间和旧算法分别核验。 |
| D123-02 | 注释与引用边界 | 显式引用的页眉页脚、脚注尾注、批注；未引用条目／作者元数据不收录，正文去重。缺失、外部、歧义、重复 ID、分隔符引用等拒绝。 |
| D123-03 | 输入与执行限制 | 无宏／链接执行；DTD、损坏、路径逃逸、条件内容、未闭合字段、输出限额等拒绝；沿用共享 ZIP/XML 限额和取消机制。 |
| D123-04 | 格式回归 | 三组原生 Go 顶层测试，共70条通过结果，零失败／跳过：[extractors.json](evidence/docx-text/extractors.json)。含既有文本／XLSX／PPTX回归。 |
| D123-05 | 实际 HTTP／PostgreSQL | 五组顶层测试，共47条通过结果：[backend.json](evidence/docx-text/backend.json)。实际提交、扫描／副本证据、任务队列、索引、搜索、完整分页／比较、秘书及当前授权；仅对象传输使用受控合成字节。 |
| D123-06 | 旧依据与升级 | 旧 ready 记录保持原字节；旧 queued DOCX 运行旧算法，新增派生独立去重；终态正文不可改写，错误 MIME／来源不允许登记。新增正常结构升级0094，原15组业务／历史事实保持。 |
| D123-07 | Web 与共享移动端 | Web333、移动端429项全部通过；共享用例验证 DOCX／混合格式各侧提取身份，不接受用旧身份替代。四端配置的类型检查、Web／微信／抖音编译及原生 Babel 基础检查通过；当前记录均在[检查目录](evidence/docx-text)。 |
| D123-08 | 实际页面闭环 | Web及Taro H5：实际登录/MFA后搜索批注文字 → 选定当前正式 DOCX → 核对修订、引用内容及完整文字摘要 → 选定两个版本比较 → 秘书资料接口复用相同派生结果。详见[Web记录](evidence/docx-text-web/result.json)与[移动记录](evidence/docx-text-mobile/result.json)。 |
| D123-09 | 媒体与清理 | 6张页面截图、2段实际静音MP4、2个明确的截图导览GIF；逐项摘要和实际视觉复核见[media.json](evidence/docx-text/media.json)、[visual-review.json](evidence/docx-text/visual-review.json)。每个自建浏览器上下文、页面、临时服务及合成数据库均在检查后关闭。 |
| D123-10 | 当前开发服务与发布检查 | 开发 API／通知／报表 Worker 更新为当前后端源码；只检查自有回环资源，保留原 PostgreSQL 实例和业务历史。当前源码、旧证据、scope、applicability及仓库发布检查记录见下表。 |

Taro H5 使用相同移动页面源码及真实原生 API。它不建立真实微信／抖音身份或上架、签名 Android、真实云、真实 Codex／TG、Linux隔离或生产验收结论。本轮没有实体设备、模拟器、网络性能、真实120GB专项、历史迁移／容灾或 AI 费用控制工作。

## 依据与复用

| 记录 | 内容 |
|---|---|
| [执行计划](evidence/docx-text/verification-plan.json) | 明确选定命令、最低用例数、最终输入、浏览器组及新增结构文件摘要；不声称全后端回归 |
| [最终源码检查](evidence/docx-text/final-source-check.json) | 当前执行结果与源码、实际浏览器/API、媒体、关闭状态、当前服务、旧发布字节和正常升级范围 |
| [源码清单](evidence/docx-text/source-manifest.json) | 应用源码及选定帮助脚本的逐文件摘要；独立浏览器清单同时绑定实际捕获脚本 |
| [旧证据保留](evidence/docx-text/reused-evidence.json) | 以发布提交 `fd71d5bd381b47266baee325e0fb3c09f9b978a3` 校验原始证据／媒体Git字节；121完整回归、runtime56项及Android开发APK仍是各自原提交的历史结果 |
| [开发更新](evidence/docx-text/development-update.json) | 当前后端服务准确归属、当前来源及原业务事实前后核对；最终重载时已是schema94，不重复应用升级 |
| [开发检查](evidence/docx-text/development-project-resources.json) | schema94来源一致、DOCX选择器／来源触发器、既有元数据与权限、Worker来源、独立集成关闭、临时数据库为零 |
| [当前范围](evidence/docx-text/scope-check.json) | 23 active（14 P0／9 P1）、5 excluded，既有范围／移出证据保留 |
| [验收适用性](evidence/docx-text/acceptance-applicability-check.json) | 原30 AC／72 TC的适用性映射；分类核对本身不执行或通过这些业务验收 |
| [仓库发布检查](evidence/docx-text/repository-check.json) | 中英文入口、相对链接、敏感资料、源码与发布说明检查；最终暂存检查另行只读执行 |

首次完整检查及页面流程均通过，但视觉复核发现旧比较提示仍排除批注，并出现 XML 风格批注范围名称。已修正提示与中文标记，最终代码重新执行检查、编译与页面流程；没有用早期通过结果替代最终源码结果。[原记录及源快照](evidence/docx-text/attempts/01-ui-copy-review/attempt.json)保留这一过程。首次开发只读检查与自有页面临时数据库并行时观察到1个临时库，正确返回失败；最终在页面关闭后重新检查零临时库。最初实际schema93→94升级和旧业务不变依据也保留在该目录，不把最终无重复升级的重载改称又执行了一次升级。

最终汇总检查首次发现新升级清单比较包含旧SQL和两个Go加载源文件，而当前清单只列SQL，因而保持整体失败。已将当前清单改为核对整个迁移目录的准确源文件集合；历史文件仍逐字节检查，新文件仍要求计划中的准确名称、数量和SHA-256。保留[失败记录与原核验脚本](evidence/docx-text/attempts/02-schema-inventory/attempt.json)。同次改进将独立Git字节核对限制为8个并行读取，结果保持原顺序与全部Git身份条件；最终19项汇总检查通过。应用输入、执行命令及执行时的 `run()` 实现不变，原测试记录继续保留实际运行器摘要。

## 可重复的本地检查

```text
python app/scripts/verify_scoped_increment.py --plan docs/quality/evidence/docx-text/verification-plan.json --run backend
python app/scripts/verify_scoped_increment.py --plan docs/quality/evidence/docx-text/verification-plan.json --run clients
python app/scripts/run_workbench_browser.py --feature docx
python app/scripts/run_workbench_browser.py --feature docx --mobile
python app/scripts/encode_workbench_media.py --feature docx
python app/scripts/verify_docx_text_development.py
python app/scripts/verify_scoped_increment.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_acceptance_applicability.py
python app/scripts/verify_repository_publication.py
```

执行检查使用已安装原生 Node22、Go、PostgreSQL、Edge与FFmpeg；所有凭据、依赖、数据库、日志和原始录制帧仍在Git外。已存在的当前结果需先保留到新的尝试目录；禁止覆盖已发布记录。实际浏览器步骤必须顺序执行，待全部关闭后运行开发资源只读检查。编码后要人工查看真实输出，再生成视觉复核记录；命令成功不能替代视觉查看。

## 仍需完成

GAP-04仍有PDF／OCR、完整Office创建编辑、版式／继承样式／图片、更多复杂修订与真实业务样本。GAP-12仍有更广秘书资料／写入／OA工具及真实模型质量。GAP-18仍有完整四端跨模块业务旅程、正式引用与批量／大文件生命周期验收。

所有23项继续列在[编号剩余事项表](../delivery/03-goal-gap-assessment.md)，本轮已完成的功能子项没有被当作整个工作包或生产发布完成。
