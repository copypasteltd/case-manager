# 167 独立源码审查：普通账号生命周期

审查基线：`e36ec01a7b78eed742a7c1f6c831a4114b96bcde`（增量166）。日期：2026-11-06。方法：`spec-to-code-compliance`，一个需求、逐层核对调用者、权限、事务、接口、页面及平台适配。本文只记录源码结论；没有执行测试、构建、服务、浏览器、Docker、WSL、虚拟机或模拟器，没有改动应用、旧证明或冻结包。结论仍待另一审查人独立反证。

## 1. 需求与结论

核对的单一需求：邀请开户、首次激活、本人改密、单用密码恢复、MFA及本人设备撤销，应组成有当前身份、会话、权限及审计约束的普通人工旅程，员工可经 Web／共享移动端完成，不依赖尚未配置的公司邮件服务。

**判定：partial。GAP-06不能全部归为公司配置或正式验收；至少有下列两个可明确开发的代码缺口。** 人工核实后交付邀请码／重置凭证本身已经实现，不应以缺少公司邮箱为理由重建邮件系统或阻塞普通流程。全部23个正式工作包及八个draft Skill继续开放；本文不改变范围、冻结标签或任何验收状态。

| 编号 | 结论 | 具体差异 | 级别 |
| --- | --- | --- | --- |
| AC167-01 | confirmed / absent | 规范明确要求“邀请过期可重发”，代码没有为原待激活账号重新签发邀请的命令或页面。24小时过期后，原登录标识不能沿普通流程完成开户。 | P1，功能缺口 |
| AC167-02 | confirmed / partial | 邀请创建和明文凭证再次展示只复核Actor携带的MFA时间及现有组织权限，没有在事务内复核原交互会话。HTTP已取得Actor后，该会话先被注销／撤销，后续邀请命令仍可提交；这与同模块改密／密码重置的会话重查约束不同。 | P1，撤销后的竞争窗口 |

规范依据：

- `docs/requirements/01-product-and-journeys.md:32-36`，J-01为“必须完成的旅程”，明确写“邀请过期可重发；首次激活有审计”。
- `docs/requirements/02-identity-and-permissions.md:13-15`，IAM-01—03规定邀请单用、过期、会话单独撤销、MFA及单用找回；`:55`要求确认会话未撤销，不能只依赖授权缓存；`:63-65`规定审计与业务写入及秘密排除。
- `docs/development/54-account-security-and-recovery.md:9-16`明确当前人工凭证交付和Web管理员签发入口；`:26-28`明确只接受普通交互账号、撤销与刷新一致锁序；`:43-54`描述恢复凭证签发、单用消费与当前权限。
- `docs/delivery/03-goal-gap-assessment.md:181`及`docs/delivery/05-functional-closure-163.md:12`把GAP-06普通流程描述为已有、主要等待人员／交付／管理员配置。AC167-01／02表明这一归因需要补充代码事项，不能只填真实配置后即宣布完整。

## 2. AC167-01：原邀请过期后无普通重发路径

### 2.1 完整证据链

| 环节 | 读取位置 | 实际行为 |
| --- | --- | --- |
| 初次签发 | `app/backend/internal/identity/administration.go:100-131` | `Invite`每次生成新的user和invitation ID；`:116`到期时间固定为24小时；`:119`始终INSERT新user；`:123`是全部后端唯一INSERT invitations。 |
| 登录唯一 | `app/backend/migrations/0001_identity_platform.sql:11-20` | `UNIQUE(org_id,login_key)`使原待激活账号继续占用内部登录标识，不能再次通过相同初始邀请创建另一个user。 |
| 回看／补交付 | `app/backend/internal/identity/administration.go:162-183` | `Invitation`仅SELECT `consumed_at IS NULL AND expires_at>now()`的原邀请；没有写入或延长有效期。 |
| 幂等重放 | `app/backend/internal/platform/commands.go:77-94`；`administration.go:134-154` | 原命令重放只返回旧回执；凭证装饰只对仍未到期且user仍invited的原记录派生原token。不能创建新有效期或新token。 |
| 激活 | `app/backend/internal/identity/administration.go:186-229` | `:208-209`只接受未用且未到期的邀请和invited账号；过期返回`INVITATION_INVALID`。 |
| 替代恢复 | `app/backend/internal/identity/account_security.go:233-241`；`identity/service.go:84-95` | 密码重置仅允许active账号；密码登录也拒绝invited。不能用改密／重置绕过激活。 |
| 停用 | `app/backend/internal/identity/administration.go:238-287` | 只有disable动作；将用户置disabled并消费旧邀请，没有重新激活或回到invited的普通命令。 |
| HTTP及契约 | `app/backend/internal/httpapi/administration.go:25-64`；`app/scripts/build_openapi.py:160-163,1682` | 只有GET原邀请、POST初次创建、POST公开激活和停用，无重新签发操作。 |
| Web管理入口 | `app/web/src/Administration.tsx:19,24,34-41` | 待激活成员只有“查看邀请”；初始对话框说明24小时；显示旧凭证或失效说明，没有重发命令。 |
| 所有员工消费入口 | `app/web/src/Login.tsx:20-22,54,61`；`app/mobile/src/pages/login/index.tsx:25,44,48,50`；`app/mobile/src/platform/session.ts:101` | 都只提交管理员交付的现有token，不能修复服务端已过期的邀请。 |

可达的具体失败：管理员邀请内部账号A → 24小时内员工未激活 → 管理员在成员表点击查看邀请得到不存在／过期结果 → 用原token激活被拒绝 → 再次邀请A受到组织内唯一标识约束。更换登录标识、手工改数据库或假装密码重置不是J-01要求的原账号重发，也不构成普通流程闭环。

这不依赖真实邮件、云、Bot、模型、签名、设备或公司审批政策。生产配置不能补齐缺失的应用命令。

### 2.2 可行的整批修复方案

1. 增加明确人工`POST /users/{id}/invitations/reissue`，接收准确目标业务版本、被替代邀请ID／版本及原因。沿用组织级`user.invite`，在事务中使用`RequireInteractiveSession(..., true)`；不得允许公开按登录标识申请、Agent或TG执行会话代签。
2. 保留原user ID、登录标识、角色、项目及作者历史，仅对仍为invited的同组织账号签发新的随机ID、到期时间和用途隔离HMAC凭证。拒绝active／disabled、过期的版本依据、跨组织及已激活竞争，不暗中恢复账号或分配权限。
3. 下一次正常追加迁移引入明确的邀请替代／撤销关系与不可变签发记录；不要修改旧迁移，不把“被替代”伪装为员工已经激活。事务先锁组织、原会话、目标user及准确旧邀请，原子使旧凭证失效、插入新摘要、增加适当版本并写回执／审计／Outbox。只保存摘要与非秘密元数据。
4. 提交后在当前原交互会话与权限仍有效、最新邀请仍待用时再派生明文；重复相同幂等键只返回同一非秘密回执及仍有效的同一token。重新签发新命令后，旧回执不可再次暴露旧有效token；任何已替代token不能消费。
5. Web成员表显示邀请有效／已过期／已被替代状态，增加明确“重新签发”，显示核实身份、手工可信渠道交付及到期时间。共享移动端可增加受权限控制的同一管理入口；员工现有四端“接受邀请”继续消费新token，不把凭证塞进URL或通用持久化存储。邮件自动发送不是这项修复的前置条件。
6. 整批先写完服务、正常追加迁移、契约／生成类型、Web／共享移动入口和前置fixtures，再统一验证。应覆盖原user不变、旧／新token、两次重发、失效回执、MFA过期、原会话撤销、跨组织、版本冲突、激活与重发竞争、幂等响应丢失、秘密不入审计、weapp／tt／Android普通密码入口。这里列的是待开发用例，并非已运行结果。

## 3. AC167-02：邀请未在事务中核对当前会话

### 3.1 调用与实际缺少的检查

- `app/backend/internal/httpapi/server.go:245-260`在读取、校验请求body之前通过`s.actor`取得一次Actor；`:211`调用`Identity.Authenticate`，后者在`identity/service.go:282-289`正确检查当前session未撤销。随后传递的是已取得的Actor快照。
- `app/backend/internal/platform/commands.go:50-69`对授权变更命令锁组织、查当前user状态及org/user授权版本，然后调用业务authorize。这里没有session行检查。
- `app/backend/internal/identity/administration.go:109-113`的Invite authorize只有`recentMFA(c.Actor)`与`RequireOrganization`。`:44-48`读取的是Actor中的MFA时间，没有查询当前session。
- `app/backend/internal/authz/policy.go:18-31`及当前最后替换的`app/backend/migrations/0011_project_viewer_policy.sql:3-43`表明can_access参数是用户、动作、资源、渠道及AI标志，不含session；检查状态／角色／分类不等于检查原会话存活。
- `app/backend/internal/identity/service.go:384-399`的Logout锁组织并只撤销目标session、写审计，不增加user.auth_version或organization.authorization_version。`account_security.go:124-138`的RevokeSession同样只撤销指定family并写普通事件，不增加这些版本。
- `administration.go:134-154`的凭证装饰和`:162-183`的重取原邀请码也只有Actor的MFA时间、组织权限和邀请有效性检查，没有`RequireInteractiveSession`或其他session行查询。后台无第二层session触发器／can_access补足这一点。

因此存在准确的并发顺序：请求A已通过HTTP鉴权、尚未进入Invite事务 → 用户注销该session，或另一有效会话完成撤销A所在family → A继续取得组织排他锁；user/org版本未变、权限仍有、Actor的MFA时间仍在10分钟内 → 创建邀请并返回明文凭证。**不是声称撤销后重新发来的普通请求能通过Authenticate；缺口限定于已经取得Actor快照、业务事务晚于撤销提交的请求。** 同一窗口也适用于凭证再次展示的读取。

对照：`app/backend/internal/identity/account_security.go:173-179,225-229,258-271`对本人改密、管理员签发和恢复凭证明文装饰均调用事务内`RequireInteractiveSession`；该函数在`identity/interactive.go:16-36`拒绝RunID／非普通渠道，并锁session行检查未撤销、未过期、当前user/org版本和数据库中近期MFA。这证明仓库已有可复用的修复机制，而非需要外部身份服务。

### 3.2 可行的整批修复方案

- Invite authorize先在现有组织锁下调用`RequireInteractiveSession(ctx, tx, actor, true)`，再检查`user.invite`；新重发命令沿用同一顺序，不能只重用Actor.MFAVerifiedAt。
- 回看及提交后明文装饰采用独立短事务，按组织→user／session→邀请顺序复核原交互会话、适用近期MFA、当前权限及准确活跃邀请，再在同事务中读取派生依据。当前会话一旦失效，不再展示明文；幂等回放不能绕过。
- 用受控暂停点前置fixture覆盖“HTTP取得Actor后撤销先提交”“注销先提交”“过期MFA”“代理RunID／TG”“明文装饰前失效”和“命令先提交则随后撤销仍生效”。先编写完整补丁及用例，再做统一测试，保留首次失败记录。此审查没有执行这些测试。

## 4. 已存在的普通流程与平台可达性

| 子流程 | 代码结论与真实边界 | 证据 |
| --- | --- | --- |
| 不依赖公司邮件的交付 | 已有管理员复制邀请／15分钟恢复token，经人工核实的可信渠道交付；没有声称已发邮件或群消息。 | `web/src/Administration.tsx:38-41`；`web/src/PasswordResetDialog.tsx:7-12`；`docs/development/54-account-security-and-recovery.md:16` |
| Web首次激活 | 登录页明确“接受邀请”，手工填写邀请码、新密码，成功后重新用内部账号登录，不自动授予角色。除过期重发缺口外，入口及服务存在。 | `web/src/Login.tsx:20-22,53-54,61`；`identity/administration.go:208-229` |
| 共享移动首次激活／密码恢复 | 登录页直接呈现接受邀请和忘记密码；不是必须先登录的设置子页。共享MobileClient使用公开消费接口。无缺失恢复链接或必须邮件回跳的前提。 | `mobile/src/pages/login/index.tsx:13-14,25,39,44,50,53`；`pages/login/password-recovery.tsx:9-23`；`platform/session.ts:101-105` |
| 本人改密 | 核验旧密码、要求已设置MFA时近期验证，变更auth_version、撤销全部session、旧challenge及重置凭证，保留MFA配置。Web／移动都有表单。 | `identity/account_security.go:150-195,390-407`；`web/src/AccountSecurity.tsx:41-44`；`mobile/src/pages/settings/security.tsx:38-55` |
| 单用密码恢复 | 管理员需明确动作、准确成员版本及原交互会话近期MFA；15分钟摘要凭证；消费时锁组织、复核目标及签发人状态／版本／权限；不自动登录且保留MFA。Web管理签发与四端员工消费符合现有说明。 | `identity/account_security.go:216-296,304-407`；`httpapi/account_security.go:40-62`；`web/src/PasswordResetDialog.tsx:7-12`；`web/src/PasswordRecovery.tsx:6-10`；`mobile/src/pages/login/password-recovery.tsx:9-23` |
| MFA设置／验证／恢复码 | 密码或已绑定平台身份后产生短期challenge；TOTP重放被拒绝，恢复码摘要单用且不删除验证器；恢复成功撤销旧session。Web／移动都有设置、验证及恢复码入口。 | `identity/service.go:96-118,144-268`；`identity/recovery.go:24-82`；`web/src/Login.tsx:24-43,54-63`；`mobile/src/pages/login/index.tsx:27-50` |
| 设备撤销与轮换 | 按本人family撤销，事务核对原交互会话；刷新继承family并拒绝重放；改密／重置整体失效。员工设置页可查看、分页与撤销，当前设备成功后清本地状态。 | `identity/account_security.go:46-139`；`identity/service.go:306-382`；`httpapi/account_security.go:13-38,66-70`；`web/src/AccountSecurity.tsx:18-39`；`mobile/src/pages/settings/security.tsx:14-36` |
| Web安全传输 | HttpOnly会话／刷新Cookie、公开CSRF Cookie；Cookie写入校验Origin和CSRF，混合Cookie与Bearer拒绝；成功改密／注销清Cookie。 | `httpapi/auth.go:91-143,154-163`；`httpapi/server.go:193-229`；`httpapi/account_security.go:66-70`；`web/src/api.ts:12-48` |
| weapp／tt | Taro wire按weapp→wechat、tt→douyin创建同一MobileClient；普通密码、激活、重置、MFA均走共享代码。平台一键登录另需真实AppID／AppSecret与本人既有绑定，不能把缺配置等同普通密码路径缺失。 | `mobile/src/platform/taro.ts:4-14`；`platform/miniapp-login.ts:8-23`；`identity/miniapp.go:309-367`；`docs/development/97-mini-program-identity.md:25-34` |
| Android | Metro明确优先本项目.rn变体；taro.rn用原生CPHTTP wire且device=android，共享登录／恢复／设置页面可达。原生bridge绑定配置API源，不走小程序一键登录。源码可达不等于当前APK包含新代码、真机或正式平台验收。 | `mobile/metro.config.js:23-37`；`src/platform/taro.rn.ts:1-5`；`platform/native-wire.ts:9-16`；`android/app/src/main/java/hk/copypaste/workspace/CPHTTPModule.java:15-35`；`platform/miniapp-login.rn.ts:3-7` |
| 移动凭证恢复边界 | access／refresh仅在MobileClient内存，丢失进程需普通重新登录；已写明安全持久化适配待独立审查。邀请／恢复token不放URL、通用存储。这不是缺公司邮箱，也没有凭空实现安全存储。 | `mobile/src/platform/session.ts:24-38,66-119`；`docs/development/97-mini-program-identity.md:34` |

## 5. 仍需公司决定／真实配置／正式验收的部分

| 项目 | 分类 | 处理边界 |
| --- | --- | --- |
| 真实人员身份核验、交付责任人及可信交付渠道 | 公司流程／配置 | 用户尚未提供；保留人工交付，不假设公司邮箱已存在。 |
| 备用管理员及权限分工 | 公司流程／配置＋实际核验 | 应由公司决定具体人员和权限；代码不虚构人员、不自动授予万能内容权。 |
| 唯一管理员完全失去密码、验证器及所有恢复码 | 明确尚未设计的受控运维恢复能力 | `docs/development/54-account-security-and-recovery.md:105`公开写明当前不能自助绕过；需要公司批准身份核验／保管／双人恢复制度，再设计受控实现。不能算已实现，也不能为修复普通重发而增加免MFA后台入口。 |
| 微信／抖音AppID、AppSecret、合法域名与实际绑定 | 真实平台配置／集成 | 由增量97明示；普通密码身份旅程不依赖平台身份登录成功。 |
| Android安全凭证持久化及正式平台发布 | 明确的平台能力／正式验收边界 | 现状是内存凭证，源码不含安全持久化适配；不据此宣称原生持久登录已验收，不把已移出的真机验证重新作为门槛。 |
| 全部真实组织旅程及受控正式验收 | 正式验收 | 本次仅源码审查，不提供任何“通过”执行证明，23包继续开放。 |

## 6. 搜索记录、覆盖和未执行事项

已执行只读定位：

```text
git rev-parse HEAD
rg --files app/backend/internal/identity
rg --files app/backend/internal/httpapi | rg '(auth|identity|session|member|user|security)'
rg --files app/web/src app/mobile/src | rg '(Login|login|Security|security|Members|members|Invite|invite|session|auth|Auth|account|api\.)'
rg -n '(invite|invitation|activate|recover|password|mfa|session)' app/backend/internal/httpapi
rg -n '(invitation|user.invite|user.password_reset|activation_token|InviteInput|ActivateInput)' app/backend app/web/src app/mobile/src -g '!**/*test*'
rg -n '(invitations|Invite\(|Invitation\(|Activate\()' app/backend -g '*.go' -g '*.sql' -g '!**/*test*'
rg -n '(邀请.*(过期|重新|补发|再次|失效)|过期.*邀请|重新.*邀请|invitation.*(expir|reissu|renew)|reissu.*invitation)' docs/requirements docs/development docs/operations README.md app -g '!**/*test*'
rg -n 'FUNCTION can_access' app/backend/migrations -g '*.sql'
rg -n '(session_id|sessions|RequireInteractiveSession)' app/backend/internal/identity/administration.go app/backend/internal/identity/roles.go app/backend/internal/authz
rg -n '(login/index|client.subscribe|client.authenticated|AccountSettings|me/sessions|useSession)' app/mobile/src app/mobile/config app/web/src/App.tsx app/web/src/api.ts
```

实际阅读包括上表中的完整相关函数与调用段：Activate／Invitation／Invite／Disable、ChangePassword／IssuePasswordReset／DecoratePasswordReset／CompletePasswordReset／replacePassword、Login／Enroll／VerifyMFA／RecoverMFA／Authenticate／Refresh／Logout／StepUp／RequireInteractiveSession、HTTP read／command／actor、当前can_access、Web登录与成员／安全／重置组件、共享移动登录／恢复／设置／会话状态与平台wire、Android原生bridge及Metro选择。未运行全回归或并发验证；没有把“存在测试文件”当执行通过，也没有复用旧证明来冒充此次验证。

Windows下带路径通配符的几次定位返回无效路径，已改为目录加`-g`或直接具名文件检索；早期包含不存在`docs/api`、`docs/guides`的复合搜索已用实际目录重查。否定结论以完整后端invitations写入／调用搜索与实际路由、契约、唯一约束为依据，而非这些失败命令。
