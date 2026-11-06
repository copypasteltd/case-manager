# 增量164发版验证 / Frozen delivery verification

本轮不新增业务，固定已推送163提交后完成当前四端与服务器原生打包、检查、封存。未执行完整业务回归、真机、网络性能／120GB、历史迁移／容灾或AI费用控制验证。真实服务配置由用户后续提供。

| 编号 | 实际检查 | 结果与来源 |
| --- | --- | --- |
| V01 | 服务器三原生Go程序、Web、运行时构建与129升级 | fresh；1537固定输入逐文件Git核对，真实工作区dirty标志保留 |
| V02 | 微信／抖音157文件包及新功能标记 | fresh；本地配置，未登录真实平台 |
| V03 | 当前Android JS／开发APK、源码Map与包内容 | fresh；223当前模块、21页面、10包检查通过 |
| V04 | JVM XML28条 | actual :app:testDebugUnitTest UP-TO-DATE；旧XML原时间和摘要保留，不冒称新跑28条 |
| V05 | 完整应用与全部媒体 | fresh；3838 Git源blob逐字节、5788展开成员、私有材料／越界路径／ZIP完整性核对 |
| V06 | 媒体两分卷 | 每卷／全文SHA及实际按顺序读取重组一致，7项截断、损坏、缺卷、乱序、路径、覆盖保护原生检查来自163 |
| V07 | 163业务与浏览器／媒体 | 原始proof、native10条+1包、Web527／移动553／runtime66及Web/H5媒体准确保留，没有重跑或扩大通过范围 |
| V08 | 新标签／产物分支与旧8冻结 | 只创建新引用；清单、Git blob和原冻结指针检查，SSH推送另有远端核对 |

首次检查失败保留在[inspection-initial](evidence/release-candidate-164/inspection-initial/)。旧检查器硬编码20页面，但源码已21页面且实际全在图中。修正仅涉及main两份打包／核对工具，原／新文件摘要与原因固定于[明确修正记录](evidence/release-candidate-164/inspection-only-source-adjustment.json)。冻结业务源码ZIP保留163原始工具字节，当前包装核对工具在main；业务摘要重算只代入这两份冻结工具，其余文件必须完全相同。应用、构建输入和既有媒体不变，无需重复构建或业务测试。

[精确最终核对](evidence/release-candidate-164/final-source-check.json) · [固定源码与旧冻结](evidence/release-candidate-164/source-lock.json) · [封存结果](evidence/release-candidate-164/seal.json) · [下载与使用](../releases/v0.1.0-rc.9/README.md)

All23 work packages and eight draft Skills stay open. A development APK and cross-build do not establish formal signing, Linux deployment or external integration acceptance. No GitHub Releases webpage is claimed; authorized SSH refs deliver the candidate. All owned review resources from163 are closed and private fixtures removed; developmentPG100/API and user processes are preserved.

本轮最终结果：冻结产物10项通过；现行范围12项、AC／TC适用性12项及仓库候选20项通过。源标签固定c727398，独立产物提交c0ca8b348ca7eb075ebf628c2ff5a11f20db0f3e；外部推送后按实际远端引用核对。没有改写163或更早的证据、媒体或升级。
