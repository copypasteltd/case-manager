# 仓库发布验证范围 / Repository publication verification scope

本次整理不改变业务源码、数据库迁移、依赖、八项Skill状态或生产配置。功能基线仍为增量76，后端／Web／运行时按完整相同来源复用增量74。未将复用或Gradle缓存计作新测试执行。

This publication does not change business sources, migrations, dependencies, Skill states or production configuration. The functional baseline remains increment76, with unchanged backend/Web/runtime evidence reused from increment74. Reuse and Gradle-retained results are not new executions.

| 检查 / Check | 实际范围 / Actual scope |
| --- | --- |
| [采集结果 / Capture](result.json) | 新执行原生TestProjectOverview隔离场景；真实Web创建项目／任务、开始／完成；Taro H5创建风险、转实际问题、调查／解决／关闭；源码前后相同 / Newly executed native fixture and actual UI/API commands, stable sources |
| [Web](web.json)、[Mobile](mobile.json) | 14张新截图、3段录制；各1个上下文关闭，无浏览器错误 / Fourteen new screenshots, three captures, both owned contexts closed |
| [编码 / Encoding](media.json) | 每帧与清单摘要校验、3段H.264 MP4及3个GIF、ffprobe时长／尺寸／编码 / Frame/manifest digests, actual output encoding and probes |
| [历史图 / Historical image](historical-media.json) | 既有真实受控秘书计划的顶部裁剪，保留原图与摘要 / Attributed top crop of an earlier controlled plan |
| [视觉查看 / Visual review](visual-review.json) | 全15图联系表、重点原图及三GIF首中末帧；抽样而非全像素OCR / Contact sheets, selected originals and GIF beginning/middle/end sampling |
| [开发环境 / Development](development-check.json)、[原生探测 / Probes](development-runtime.json)、[状态核对 / State](development-project-resources.json) | 新的只读检查：69项迁移、10探测、0隔离测试库、集成关闭、八项draft；未重启／新增日常服务 / Fresh read-only inspection without restarting services |
| [仓库检查 / Repository preflight](repository-check.json) | 选定文件与暂存区、双语覆盖、本地Markdown链接、敏感模式、忽略规则、体积、媒体摘要及功能来源复算 / Selected/index files, language coverage, links, high-confidence secret patterns, ignores, sizes, media and source proofs |

初始全量导入的`git diff --cached --check`发现15个既有文件的3440处空白提示，主要为原始构建日志与官网SVG，另有四处既有应用文件。为保留原证据摘要，没有格式重写这些旧文件；[初始空白基线](initial-whitespace-baseline.json)记录准确文件、数量和原摘要。本轮新增代码及指南另做范围明确的空白检查。首次提交之后继续对真实增量执行Git空白检查。

The full initial-import whitespace check reports 3,440 findings in 15 pre-existing files, predominantly raw build logs and official SVGs, plus four existing application files. Their bytes remain unchanged to preserve evidence. The [baseline](initial-whitespace-baseline.json) records exact paths, counts and hashes. New publication code/guides receive a separate scoped whitespace check; subsequent commits use Git's incremental check.

失败尝试保留在attempts：第一次不需要的管理员夹具、第二次无权限页面等待、第三次历史本机APK链接。前两次最终被正确清理，不扩权；第三次将本机APK链接改为明确“未纳入Git”的原路径文字，保留构建结论和摘要。最终记录取自真实完成结果，不抹掉尝试。

The attempts directory retains an unnecessary admin fixture, an incorrect wait on a permission-denied page, and historical local-only APK links. The first two were cleaned up without granting privileges. The third replaces unavailable Git links with explicit local-only paths while retaining original build results and hashes.

全部素材使用隔离合成数据；H5不是设备验收，元数据不是云传输证明，受控工具回执不是模型质量验收。真实模型／云／TG／平台／Android设备、120GB／2TB、安全、性能及生产恢复仍按[原28项GAP](../../../delivery/03-goal-gap-assessment.md)推进。

All media uses isolated synthetic data. H5 is not device acceptance, metadata is not cloud-transfer proof, and controlled receipts are not model-quality acceptance. The original production/external gates remain open.
