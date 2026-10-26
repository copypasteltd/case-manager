# 增量77：仓库整理、双语指南与实际演示

本轮按用户授权将现有实现、文档和可核对演示组织为 `copypasteltd/case-manager` 的初始提交。完整平台 Goal 继续执行；不通过仓库发布扩大已有业务能力或把开发验证改称生产验收。

## 1. 交付组织

| 内容 | 位置 | 作用 |
| --- | --- | --- |
| 双语入口 | [根README](../../README.md) | 方案概览、实际演示、启动与验证范围 |
| 完整指南 | [中文](../README.zh-CN.md)、[English](../README.en.md) | 需求、架构、模块、员工流程、原生开发、四端构建、生产存储与未放行项 |
| 维护约束 | [贡献与同步](../contributing.md)、[根AGENTS](../../AGENTS.md) | 后续代码／文档／媒体／更新记录同步提交推送；禁止本地虚拟化和遗留浏览器 |
| 更新说明 | [CHANGELOG](../CHANGELOG.md) | 双语初始发布及功能基线说明 |
| 实际媒体 | [媒体目录](../media/README.md) | 15张PNG、3段MP4、3个GIF及来源／范围 |
| 自动工具 | [采集](../../app/scripts/run_repository_showcase.py)、[编码](../../app/scripts/encode_repository_showcase.py)、[仓库检查](../../app/scripts/verify_repository_publication.py) | 不录制凭据、完整来源摘要、逐帧摘要、FFmpeg实际编码、提交边界检查 |

既有原生依赖、缓存、私有配置与工具链保留在忽略目录，不移动成仓库依赖。代码仍在app/；文档、历史与媒体仍在docs/。保留原字节的gitattributes避免Git行尾转换使历史来源摘要失效。

## 2. 实际采集与失败记录

使用原有 `TestProjectOverview` 原生数据库场景准备合成业务事实，Web实际创建新项目和任务并开始／完成，Taro实际创建风险、转真实问题并调查／解决／关闭。后台验证实际ID与状态。Web与H5顺序占用自己的5189端口，原始帧留在忽略目录；所有浏览器、临时服务、夹具和隔离数据库在finally中清理。

第一次准备错误地启用了不需要的管理员移动夹具，原测试在缺失管理员MFA上失败；已[归档](../quality/evidence/repository-showcase/attempts/01-unneeded-administrator-fixture/result.json)，移除不适用的夹具选项。第二次新项目流程通过，但采集器对无秘书权限身份错误等待页面标题；已[归档](../quality/evidence/repository-showcase/attempts/02-secretary-permission-boundary/result.json)，改为读取实际拒绝信息，不增加身份权限。

最终[采集](../quality/evidence/repository-showcase/result.json)两个阶段通过，来源前后相同；Web／移动各关闭一个上下文，所属浏览器、临时服务、私有夹具及测试数据库已清理。来源摘要为 `418ae923cabb933fb1ef42c27d543a824aad0d0126727969804a552178a626d4`，覆盖实际应用、后端契约及采集脚本。

## 3. 编码与视觉范围

原始JPEG按实际采集间隔编码，先核对每帧与清单摘要。MP4为无声H.264、10fps；GIF为6fps、128色，桌面960宽、移动390宽。三个MP4分别约4.2／16.6／6.9秒，GIF只对较长业务浏览片段约1.09倍加速。编码与ffprobe见[media.json](../quality/evidence/repository-showcase/media.json)。

15张截图中，14张来自本次实际采集。秘书准确计划图是既有完整受控流程顶部裁剪，保留原图、摘要和坐标，不表示本次模型运行。已查看全体截图缩略联系表、重点原尺寸截图及三GIF首／中／末帧。此为明确抽样检查，不宣称逐像素OCR。说明与记录见[媒体目录](../media/README.md)。

## 4. 验证与发布边界

本轮业务源码、Go／SQL、Web／移动业务、运行时、依赖与Skill均未改。增量76与74的功能基线只有在完整当前来源摘要相同时复用；本次不重新执行全部146个后端顶层测试或重复Android构建。新采集实际运行原有项目概览集成场景及实际浏览器操作。

另做当前原生开发环境只读核对、候选／暂存区敏感信息与忽略规则、单文件体积、双语覆盖、Markdown链接、媒体摘要、源码稳定性与清理检查。仓库检查记录存放在[证据组](../quality/evidence/repository-showcase)。不记录或提交服务器私钥、模型／TG／云凭据、开发数据库或原始帧。提交后对照远端main和本地HEAD验证同步，不强推。

完整四端业务矩阵、真实模型与隔离、八项Skill发布、真实云／TG／平台／设备、120GB／2TB、安全、性能与生产恢复仍按原28项GAP推进。本轮记录发布组织与使用逻辑，不将任何生产GAP宣布关闭。
