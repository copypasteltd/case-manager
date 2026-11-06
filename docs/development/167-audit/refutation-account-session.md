# AC167-02：邀请命令及明文展示的原会话竞争独立反证

日期：2026-11-06。**全部应用、SQL与既有规格证据固定为 `e36ec01a7b78eed742a7c1f6c831a4114b96bcde`，通过 `git show e36ec01:<path>`／`git grep ... e36ec01 -- <path>`读取。** 当前工作目录正在开发的新代码没有被当作原基线证据。只新增本报告；未运行测试、构建、服务、浏览器、Docker／WSL／虚拟机／模拟器，未修改应用或旧证明。

## 1. 结论与精确范围

**独立反证结果：AC167-02成立，源码置信度高；未找到原基线中足以阻止所述顺序的现有屏障。** 原始HTTP鉴权能够拒绝已经撤销会话发来的新请求；问题限定为：一个请求先取得Actor快照，原会话注销／设备撤销先提交，该请求之后才进入邀请创建或明文邀请展示的业务检查。

组织／用户锁确实存在，不能说“邀请没有锁或重新鉴权”。实际缺的是原交互session的事务内复核与行锁。注销及本人设备撤销只更新sessions，不增加这里比较的user.auth_version／organization.authorization_version，故它们先提交后，仍有效的账号／角色／旧Actor近期MFA不足以拒绝这个在途请求。

**锁描述修正：** `identity/locks.go:11–13` 的 `lockOrganization` 使用 **`FOR KEY SHARE`**，不是组织排他锁。Invite与设备RevokeSession通过`AuthorizationMutation=true`使`Commands.Do`使用 **`FOR UPDATE`**。这些锁可以决定重叠事务的先后，却不会在撤销已经提交之后自动使旧Actor无效。此修正不推翻候选。

规格依据为原基线 `docs/requirements/02-identity-and-permissions.md:14–16,55–57`：设备可单独撤销；必须确认会话未撤销、不能只依赖授权缓存；停止受影响尚未提交操作。`docs/development/54-account-security-and-recovery.md:26–28,42–46` 另明确交互会话及一致锁顺序；同模块已有可复用的会话复核能力。

## 2. 对既有屏障逐项反证

以下行号均指原基线；后端短路径相对 `app/backend/internal/`。

| 编号 | 可能已经存在的屏障 | 实际读取与判定 |
| --- | --- | --- |
| AS-01 | HTTP鉴权一直锁住会话直到业务提交 | `httpapi/server.go:193–229,245–260`调用`Authenticate`后返回Actor，再解码请求和调用业务。`identity/service.go:277–293`通过pool执行普通SELECT，没有事务或session行锁延续到Invite。不能覆盖鉴权后再发生的撤销。 |
| AS-02 | Cookie写入的CSRF检查就是最终会话复核 | `httpapi/server.go:221–225`、`identity/service.go:295–304`确实再查询`revoked_at IS NULL`；但该SELECT仍在业务事务之前，不持有session锁。选择“CSRF也已完成→撤销提交→Invite开始”的顺序即可，故不是绕过CSRF。移动bearer路径亦不依赖该检查。 |
| AS-03 | AuthorizationMutation自动增加权限版本，撤销后旧Actor必失效 | `platform/commands.go:50–69`只因该标志改组织锁为FOR UPDATE，并比较组织版本、用户状态／auth_version，再调用authorize。`:129–137`提交时也没有自动增加组织或用户版本。标志并不等于版本递增。 |
| AS-04 | Invite authorize重查了原会话 | `identity/administration.go:44–48,108–123`仅调用`recentMFA(Actor)`和`authz.RequireOrganization(tx,Actor,"user.invite")`。MFA只检验Actor携带时间；本路径没有session查询／RequireInteractiveSession。 |
| AS-05 | RequireOrganization／can_access内部检查了原session | `authz/policy.go:18–31`仅传org、user、动作、资源、分类、channel及是否AI，不传session_id；最新`can_access`定义为 `app/backend/migrations/0011_project_viewer_policy.sql:3–43`，检查active用户、策略、有效角色及范围，没有sessions查询。因此角色仍在时不能区分被撤销的那一会话。 |
| AS-06 | Logout改变安全版本使AS-03失败 | `identity/service.go:384–399`只在KEY SHARE组织锁下UPDATE准确sessions.revoked_at、写authAudit并提交；不改user／org版本。会话行保留，账号及角色不变。 |
| AS-07 | 设备RevokeSession会改变账号／组织版本 | `identity/account_security.go:114–139`确有当前交互session复核、组织FOR UPDATE与目标family行锁；但只UPDATE目标family的revoked_at并返回事件。另一有效会话B撤销同一用户的A，不改变A已携带的user／org版本。 |
| AS-08 | 业务audit／outbox会拒绝已撤销的session | `platform/commands.go:103–137`写审计、Outbox、幂等结果并提交，没有原session状态检查。原始SQL与后续迁移搜索未发现挂到sessions／users／invitations的撤销→安全版本触发器；`0001_identity_platform.sql:118–131`的audit触发器是UPDATE／DELETE不可变保护，不能补业务会话校验。 |
| AS-09 | 提交后明文装饰有第二次原会话检查 | `httpapi/administration.go:47–61`在Invite返回后调用Decorator；`identity/administration.go:134–154`只检查Actor的近期MFA、当前组织权限、准确邀请未使用未过期且目标仍invited，再派生token。使用pool独立查询，没有原session查询及跨查询事务／行锁。 |
| AS-10 | GET“查看邀请”或统一响应再次验证session | `httpapi/administration.go:25–27`→`identity/administration.go:162–183`也是组织动作检查和有效邀请读取后调用同一Decorator。`httpapi/server.go:276–289,138–148`只在入口鉴权；返回时仅序列化、响应契约校验和写响应，无再次认证。 |
| AS-11 | 改密／恢复采用的更强边界已被整个identity模块统一应用 | 该边界存在但需显式调用：`identity/interactive.go:16–37`检查原session_id、渠道、未撤销／未过期、当前user／org版本和数据库近期MFA，`FOR SHARE OF s`保持到调用方事务结束。`account_security.go:173–178,225–229,258–296`对改密／恢复签发／恢复明文装饰使用它；原Invite／Decorator未调用，不能将模块其他方法的保护自动推定给这里。 |

## 3. 两个可达顺序（静态推导，未执行）

**创建分支：** A拥有`user.invite`及近期MFA，HTTP认证／必要CSRF检查完成；在A尚未进入Invite事务时，A注销，或同一员工的有效会话B撤销A所在family并提交。A之后进入Invite：组织与用户版本、active状态、组织角色和Actor MFA仍满足检查；邀请INSERT、审计与Outbox可提交。Decorator也不查询A是否已撤销，可返回新邀请明文。

**单独展示分支：** 原有效邀请已存在。A的GET请求完成HTTP认证后，B撤销A并提交；GET业务与Decorator之后再读取当前角色和邀请，仍能派生明文。另一个等价间隙是Invite已提交而Decorator尚未开始时撤销A；这里创建已完成无需回滚，但新的秘密展示步骤仍需原会话检查。

上述不假定账号停用、角色被撤销或密码已修改；这些操作可能改变版本／权限并被现有机制拒绝，不能拿来替代“只撤销单个session”反例。也不把“明文检查／事务先完成，响应字节在网络中时才注销”列为失败；已产生的在途响应无法被撤回，这不是本候选的修复目标。

## 4. 完整修复的必要边界（本报告不实现）

- Invite与新增重发命令，在已有组织／用户锁后显式调用`RequireInteractiveSession(...,true)`，再检查当前组织动作；原session行锁维持到命令事务提交。幂等重放同样先复核当前原会话。
- 明文展示以独立短事务复核原交互会话、数据库近期MFA、当前组织权限及准确活跃邀请，派生后提交；GET与提交后Decorator共享同一实施路径。组织KEY SHARE与session SHARE可以与Logout组织KEY SHARE共存，但后续session UPDATE须与SHARE锁排序，不能只新增一个无锁SELECT。
- 保持组织→用户／session→邀请的锁顺序，保留旧会话撤销、已有邀请／审计／幂等历史，不通过重建session、延长MFA或自动重试新邀请掩盖竞争。
- 整批源码完成后再统一验证：入口鉴权后Logout先提交、另一会话family撤销先提交、明文装饰前撤销、原会话先完成授权及提交的合法相反顺序、幂等回放、当前MFA、跨用户／渠道及错误回滚。此处只是待测输入，不记为运行通过。

## 5. 搜索与证据边界

实际搜索均固定git对象：Authenticate／Logout／RevokeSession／RequireOrganization／RequireInteractiveSession／Commands.Do函数定义；HTTP认证、CSRF、command／read／respond调用；全部迁移的`CREATE (OR REPLACE )?FUNCTION can_access`（仅0003／0008／0011，完整读最新0011）；`ON sessions|ON invitations|ON users`（只有索引命中，没有该类触发器）；邀请相关路径的`RequireInteractiveSession|current_setting|set_config.*session|SET LOCAL.*session`（未发现隐藏身份传递）。曾猜测的`authz/authorization.go`在该git对象不存在，已改为实际`authz/policy.go`；路径错误不作为能力缺失证据。

没有将源码推导说成已运行的并发重现，没有读取正在开发的新应用内容来背书旧候选，也没有对root的新补丁或其测试状态作结论。23个正式工作包、八个draft Skill、冻结产物和范围排除保持原状态。
