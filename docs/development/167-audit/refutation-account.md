# J-01 过期邀请重发：独立反证记录

固定源码：`e36ec01a7b78eed742a7c1f6c831a4114b96bcde`。日期：2026-11-06。本次为另一审查者提出的候选问题作独立反证；仅阅读源码、契约和规格，未执行测试、构建、服务、浏览器或数据库变更。未修改应用或旧证据。

## 要求与判断

原始规格 `docs/requirements/01-product-and-journeys.md:36`，必须完成的旅程J-01：

> “管理员邀请 → 一次性激活 → 设置认证 → 分配角色/项目”；“无公开注册；邀请过期可重发；首次激活有审计”。

**独立反证结果：候选成立；过期邀请的重新签发能力absent，J-01整体partial，源码置信度高。** 没有找到可供普通管理员在保留同一个账号身份和内部账号名称的情况下，为过期未激活账号签发新有效邀请的实际命令。可确认的首次邀请／有效邀请再查看／一次性激活均不能填补过期后的分支。该缺口不依赖公司制度、云账号、正式平台资料、真机或网络性能。

本判断不是声称“账号系统没有实现”，也不是将其他23个工作包改记完成；只确定J-01的一条异常旅程尚缺代码。

## 尝试推翻候选的实际路径

| 编号 | 可能的替代实现 | 读取的真实执行位置 | 为什么不能推翻候选 |
| --- | --- | --- | --- |
| AR-01 | 对相同账号再调用邀请接口 | `app/backend/internal/httpapi/administration.go:47–61` → `identity/administration.go:100–131` | 每次新请求都先随机生成新user／invitation ID，再INSERT新users。没有查找已有invited账号并续发的分支。原邀请24小时有效（`:116`）。 |
| AR-02 | 让新邀请复用同一login_key | `app/backend/migrations/0001_identity_platform.sql:11–20`；`internal/platform/database.go:45–49` | `(org_id,login_key)`唯一约束覆盖invited和disabled；重复INSERT触发409 `DUPLICATE_RESOURCE`。后续迁移搜索未发现删除或收窄这一唯一性约束。 |
| AR-03 | “查看邀请”其实会续期 | `internal/httpapi/administration.go:25–27` → `identity/administration.go:162–183` | GET只读取`consumed_at IS NULL AND expires_at>now()`的最新记录；过期无记录可返回。没有INSERT／UPDATE有效期。 |
| AR-04 | DecorateInvitation动态生成token等于重发 | `identity/administration.go:93–99,134–154` | 只对仍未使用、未过期且账号invited的原记录，用原ID／原ExpiresAt重建token并校验原token_hash。没有刷新日期或增加邀请。过期回执不会获得新的有效token。 |
| AR-05 | 重放创建请求会续期 | `internal/platform/commands.go:71–98` 与 `httpapi/administration.go:52–60` | 原幂等键成功后直接返回原receipt，业务fn不执行；Decorator仍检查原邀请是否有效。换幂等键则返回AR-02重复账号冲突。 |
| AR-06 | 可以用已过期token直接激活 | `identity/administration.go:186–229` | Activate锁组织与邀请／账号，要求`expires_at>now()`、未消费、`u.status='invited'`；不符返回`INVITATION_INVALID`。这个拒绝正确，不能靠放宽期限替代新邀请。 |
| AR-07 | 密码重置可以替代激活 | `identity/account_security.go:225–241`，重置token后续使用`:283–285` | 管理员签发重置要求账号`active`；invited返回`ACCOUNT_NOT_ACTIVE`。不能为过期invited账号完成入用。 |
| AR-08 | 先停用再新建／重新启用 | `identity/administration.go:238–285`；`0001_identity_platform.sql:19` | 停用更新原users为disabled、保留login_key并消费未使用邀请；没有删除账号或释放login唯一槽。未发现重新启用／重邀请分支。不得把直接改生产SQL当普通产品能力。 |
| AR-09 | 初始化管理员工具可处理 | `identity/bootstrap.go:32,57–68` | Bootstrap在组织已存在时拒绝，不会为原组织补发邀请，也不能作为日常业务权限旁路。 |
| AR-10 | Web按钮调用隐藏续发命令 | `app/web/src/Administration.tsx:19,24,34–41` | invited行按钮“查看邀请”调用AR-03的GET；新建表单仍POST `/users`，只提交login_key／display_name。Dialog无过期签发命令。 |
| AR-11 | 共享移动端或原生另有续发实现 | `app/mobile/src/platform/session.ts:101`、`pages/login/index.tsx:25`；全app邀请符号与写操作搜索 | 共享移动有接受邀请→`/auth/activate`，未发现重新签发邀请调用。Java／Kotlin无邀请写入旁路。该判断只涉及此次续发要求，不由搜索推定整个移动成员管理的验收状态。 |
| AR-12 | 契约定义了未注意到的写接口 | `app/scripts/build_openapi.py:144,160–161,1682–1683`；`internal/contract/openapi.json:5621,5707,5788–5790`；`app/packages/contracts/src/api.d.ts:1044–1060` | 当前仅`inviteUser`与`getUserInvitation`，GET无重新签发命令；实际路由亦一致。 |

表中后端`identity/`／`httpapi/`／`platform/`均相对 `app/backend/internal/`；迁移相对 `app/backend/`。

## 具体可达的失败状态（静态推导，未运行复现）

管理员正常邀请login L，创建user U与24小时邀请I；员工未及时激活，I过期而U仍invited。之后管理员查看I只会走有效期过滤；重发原请求只是旧回执，新请求又无法绕过U的同组织login唯一性；员工激活旧token被正确拒绝。U没有普通应用内恢复这一入用步骤的路径。

通过另外一个login创建另一个人、放宽过期激活条件或手工修改数据库，均不兑现“为该账号重发邀请”，还会改变身份／安全语义。

## 搜索记录与未验证范围

- 对 `app/backend` 全部非测试Go／SQL搜索`invitation|invitations|resend.inv|reissue.inv|renew.inv`，业务命中仅`internal/identity/administration.go`、`internal/httpapi/administration.go`及两份原始身份迁移。所有实际邀请写操作只有：首次INSERT（`:123`）、激活消费（`:216`）、停用消费（`:283`）。没有更新expires_at或另一个邀请INSERT入口。
- 全app Go／SQL／TS／TSX／Java／Kotlin搜索邀请符号，额外命中Web管理页和共享契约；移动接受邀请单独通过`auth/activate`及`activate(`定位。`resend/reissue/renew/re-invite/extend`与邀请／中文重发的联合搜索无匹配。
- 全部迁移搜索`UNIQUE.*login_key|users_org_id_login_key|DROP.*login_key`仅找到原始唯一性定义；无删除users、改login_key或续期邀请的普通写操作路径。密码重置与Bootstrap额外阅读，以排除错误替代方案。
- 检查脚本`app/scripts/check_administration.mjs:71–93`只展示邀请创建与及时激活的既有用例源码，未执行；不能把它当过期重发证据。不存在的猜测路径`identity/administration_integration_test.go`、`platform/command.go`、`app/api`等只产生工具路径错误，已通过实际文件搜索修正；这些错误不用于支持能力缺失。
- 本报告没有执行数据库唯一冲突、并发激活／续发、幂等重放或客户端行为；上述是与SQL及控制分支逐一对应的静态推导。仓库外手动运维代码或未交付私有服务不作为已存在产品能力。

## 完整补齐的最小边界（未实现）

新增明确的人工重新签发命令，固定原user ID、expected_version、准确旧invitation依据及原因。近期MFA、当前真实交互会话、`user.invite`组织权限、组织／账号／邀请锁和一次性激活竞争规则须保持；仅invited账号适用，active／disabled拒绝。复用当前统一命令事务／幂等／审计／Outbox，重放只返回同一次新邀请。

新建邀请行、保留旧过期事实及旧token失效，不原位改旧expires_at；若增加独立撤销／继承字段，使用追加迁移。明确并发续发、已激活、管理员撤权、原邀请有效时的产品策略。token仍仅在当前权限检查后的响应临时生成，不写明文数据库、审计或录屏。读取界面应能获得过期状态和准确basis，不能把过期GET的404伪装成“没有账号”。

更新后端契约、Web及必要共享移动成员操作入口；接受邀请仍走原一次性激活路径。整批源码完成之后再统一覆盖过期重发、旧token拒绝、新token仅使用一次、同账号保留、幂等、并发激活／续发、当前权限/MFA、事务回滚及页面提交读回。这些用例此刻均未执行，不能记为通过。
