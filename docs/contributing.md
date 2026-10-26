# 开发、文档与仓库同步 / Development and repository synchronization

[中文指南](README.zh-CN.md) · [English guide](README.en.md) · [更新记录 / Changelog](CHANGELOG.md)

本规则落实用户要求：后续完成更新时，同步相应代码、文档、演示与更新说明并推送到本仓库。推送目标为 `git@github.com:copypasteltd/case-manager.git`，不需要为同一已授权范围重复询问。遇到新的不可逆外部动作或真实环境配置，按其实际授权范围处理。

The user has authorized publishing completed updates to this repository, including corresponding code, documentation, demonstrations and update notes. The remote is `git@github.com:copypasteltd/case-manager.git`. Do not repeatedly request permission for the same authorized scope. Distinct irreversible external actions and real environment configuration still follow their actual authorization scope.

## 1. 目录与环境 / Layout and environment

实现放在 `app/`；新增协议、需求、维护说明、证据和媒体全部放在 `docs/`。根 README 是双语入口，完整语言指南位于 `docs/README.zh-CN.md` 与 `docs/README.en.md`。仓库约束以根 [AGENTS.md](../AGENTS.md)及用户最新要求为准。

Keep implementation in `app/` and protocols, requirements, maintenance notes, evidence and media in `docs/`. The root README is bilingual; full guides live in the two language-specific files. Follow root [AGENTS.md](../AGENTS.md) and current user instructions.

不启动本地 Docker、WSL、虚拟机或模拟器。不调度子代理。浏览器检查结束关闭自己创建的页面、上下文及浏览器；临时进程与数据库先确认归属再清理。不能关闭用户原有服务。PowerShell 脚本使用 UTF-8，私有配置不得写进可见日志。

Do not launch local Docker, WSL, VMs or emulators, or delegate to subagents. Close owned pages, contexts and browsers after checks. Confirm ownership before removing temporary processes/databases and preserve unrelated services. Use UTF-8 in PowerShell and keep private configuration out of visible logs.

## 2. 每次功能更新 / Each functional update

1. 先读对应需求与开发协议，选择原有 GAP 编号；不要把尚未完成项改名消失。Read the relevant requirements/protocols and preserve stable gap IDs.
2. 修改原业务实现，保持当前授权、固定依据、版本、幂等、审计与真实回执。Preserve current authorization, fixed evidence, versions, idempotency, audit and actual receipts.
3. 根据影响范围执行验证，记录准确命令、源码摘要、结果、失败尝试和外部边界。Run scoped checks and record commands, source digests, results, failures and external limits.
4. 同步中文与英文使用指南、相关模块协议、索引、实施台账和 `docs/CHANGELOG.md`。Update both language guides, affected protocols, index, implementation ledger and changelog.
5. 用户可见行为变化时更新对应截图／录屏／GIF 和媒体来源记录；未变化的媒体经来源与语义核对后可保留，不为纯文字修改重录全部界面。Refresh affected user-visible media and provenance; unchanged media can remain after source/semantic review.
6. 做仓库检查、审阅实际暂存区，再提交并推送。Run repository checks, inspect the index, then commit and push.

## 3. 按影响验证 / Verification by impact

| 改动 / Change | 适当检查 / Appropriate checks |
| --- | --- |
| Go、SQL、权限或契约 / Backend, SQL, authorization, contract | 相关真实数据库场景、完整后端批次按风险要求；OpenAPI生成契约；各端类型检查 / Relevant native database scenarios, full batches where needed, contract generation and client types |
| Web业务 / Web workflow | 相关单测、构建、实际 API 浏览器流程、受影响页面布局和媒体 / Tests, build, actual API flow, affected layout/media |
| Taro公共业务 / Shared mobile | 单测、类型、双小程序编译、实际 H5 流程；受影响的 Android 开发包 / Tests, types, both builds, actual H5 flow and affected APK |
| Android原生 / Android native | 原生单测与包检查、开发包；需设备行为则保留真机验收 / Native tests/inspection/build; device acceptance for device behavior |
| SDK、Skill、工具 / Runtime, Skill, tool | 运行时测试、包校验、原业务真实回执与越权／冲突／撤回 / Runtime/package checks and actual business/authorization cases |
| 文档／媒体／仓库 / Docs, media, repository | 链接、双语覆盖、媒体摘要、敏感信息、忽略规则和体积 / Links, bilingual coverage, media digests, secrets, ignores and sizes |

只在完整相关来源相同并且原证据范围足够时复用；缓存的 Android 结果不能写成重新执行。H5、模拟存储、合成数据或未启用模型的证据不能升级成云／设备／生产通过。历史记录保留其原日期和结论，新轮验证写入新证据组，不覆盖失败或历史结果。

Reuse evidence only when complete relevant sources match and its original scope is sufficient. Cached Android results are not new executions. H5, controlled storage, synthetic data and disabled-model evidence cannot become cloud/device/production acceptance. Preserve historical dates and failures; write new verification to a new evidence group.

## 4. 提交与推送 / Commit and push

原始录制帧、凭据、本地配置、`.env`、`.local`、`.tools`、`node_modules`、构建缓存、数据库、签名密钥和 APK 均不提交。`.env.example` 与 Android Gradle wrapper 是明确的源码依赖例外。媒体压缩后提交；本次单个媒体不到 1 MB。不得将凭据放入截图、录屏或 GIF。

Do not commit raw capture frames, credentials, local configuration, `.env`, `.local`, `.tools`, `node_modules`, build caches, databases, signing keys or APKs. `.env.example` and the Gradle wrapper are intentional source prerequisites. Commit compressed media; current individual media files are below 1 MB. Credentials must not appear in screenshots or recordings.

从仓库根执行，提交信息按实际改动填写：

From the repository root, using a message that describes the actual change:

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_repository_publication.py
git status --short
git add -- README.md AGENTS.md .gitattributes .gitignore app docs
git diff --cached --stat
git diff --cached --check
python app/scripts/verify_repository_publication.py --staged
git commit -m "Describe the completed code and documentation update"
git push origin main
git status --short
git ls-remote origin refs/heads/main
```

若在功能分支，推送对应分支并保留审阅流程；不要强推 main。远端新增提交时先 fetch，审阅并集成后再推，不能覆盖他人工作。推送后确认远端提交与本地一致、工作区无遗漏；失败时保留本地提交并说明实际原因，不谎报同步成功。

On a feature branch, push that branch and retain the review workflow. Never force push main. Fetch, inspect and integrate incoming work before pushing. Verify remote/local commit equality and a clean worktree afterward. If push fails, preserve local commits and report the actual cause instead of claiming synchronization succeeded.

`.gitattributes` 保留现有文件原字节，防止行尾转换破坏证据来源摘要。不要为了格式整理统一改写历史证据或源码的行尾；确需变更时重新执行受影响验证并留下新来源记录。

`.gitattributes` preserves exact existing bytes so line-ending conversion does not invalidate source-bound evidence. Avoid mass line-ending rewrites; intentional changes require appropriate fresh verification and provenance.

首次导入包含既有原始日志、SVG及少量旧源码空白提示，其例外已记录为[初始基线](quality/evidence/repository-showcase/initial-whitespace-baseline.json)，本轮新增代码／指南单独通过检查。后续提交仍执行上面的增量检查，不把初始基线当作新增空白问题的通用豁免。

The initial import retains historical log/SVG/source whitespace recorded in the [initial baseline](quality/evidence/repository-showcase/initial-whitespace-baseline.json); new publication files pass a separate check. Continue incremental checks on later commits; the baseline is not a blanket exemption for new findings.
