# GAP-06 — 独立反证：重发后的停用与角色变更原会话

独立复核者原任务为GAP-05，未参与两项候选的形成。基线`930cdde`，结论来自审查时实际源码／SQL；未运行测试、构建、浏览器或服务，未改应用代码。尚未存在的GAP-06作者报告不作为证据；直接复核候选本身。

## F-G06-01 — 重发过邀请的账号不能正常停用：保留

要求：`docs/requirements/02-identity-and-permissions.md:13` IAM-01明确员工可处于invited/active/disabled，并要求停用保留历史。第133号普通升级同时保留重发前邀请的独立撤销事实。

真实路径：`httpapi/administration.go:79–80` → `identity/administration.go:205–265` Disable → `platform/commands.go:22–42,102–105` 同一事务，遇错误整体回滚。

- `identity/invitation_reissues.go:55–58`将前任邀请设置`revoked_at/revoked_by`，条件为`consumed_at IS NULL AND revoked_at IS NULL`，因此前任留下非空revoked及空consumed；`:60–70`生成同用户的新邀请并增加用户version，旧行仍保留。
- `migrations/0133_invitation_reissues.sql:10`原文为`CHECK (consumed_at IS NULL OR revoked_at IS NULL)`，禁止同一邀请同时记为已消费和已撤销。
- 当前`identity/administration.go:250`实际SQL为`UPDATE invitations SET consumed_at=coalesce(consumed_at,now()) WHERE org_id=$1 AND user_id=$2`，没有跳过已撤销行。它会同时给该用户的前任已撤销邀请写入consumed，违反上述CHECK；该函数立即返回错误，之前的用户停用／会话撤销也随Commands事务回滚。

**反向证据及排除：** 首次邀请未发生重发时没有已撤销前任，因此普通停用用例可通过；这不覆盖候选。Activate仅消费所选有效新邀请（administration`:176–183`），不会移除或清空前任的revoked事实，所以“先重发再激活再停用”仍受影响。查询所有迁移中的`invitations`只得到建表、过期索引和133的新增约束，无自动跳过已撤销行的触发器。此前167文字声称有过滤条件不能代替当前SQL；当前Disable的管理员计数查询`:238`反而重复了`rb.revoked_at IS NULL`，不能作用于后续邀请UPDATE。

**源码确认的触发条件：** 有另一有效管理员，目标曾被重发邀请，使用目标当前用户version进行合法停用。此为SQL语义与调用链证明，尚未做运行时复现；后续整批测试应覆盖未激活／已激活、重发多次、历史撤销事实和原子回滚边界。修复应仅消费未撤销邀请，保留旧撤销事实，不修改已应用133迁移或历史证据。

## 第二候选 — 角色授予／撤销只核对捕获MFA，不核对原会话：保留，限定竞态范围

要求及范围：IAM-02（同文`:14`）要求不同设备可独立撤销会话，IAM-03要求管理员MFA，组织配置表`:30`将`role.assign`归为敏感变更二次认证。这里保留的是敏感写入时仍应具有原有效人类会话的缺口；不是声称一个已撤销令牌能够重新通过HTTP鉴权。

- `identity/roles.go:138–144,203–209`的CreateBinding/RevokeBinding授权回调只有`recentMFA(c.Actor)`和`authz.RequireOrganization(...,"role.assign")`。`administration.go:47–52`的recentMFA只检查捕获Actor里的时间戳，不查sessions，也不锁定原会话。
- `platform/commands.go:49–72`在组织／用户锁下复核active及org/user授权版本，再调用上述回调；没有原sessions检查。`authz/policy.go:19–31`传给can_access的参数无SessionID；当前定义`migrations/0011_project_viewer_policy.sql:3–48`仅查询账号／策略／角色／范围，不能识别哪一会话已撤销。角色绑定及通用审计迁移也没有以原会话为依据的INSERT拦截器。
- `identity/service.go:384–398` Logout、`account_security.go:113–140` RevokeSession会持锁撤销原session/family，但不改变用户auth_version或组织authorization_version。若它们先完成，先前捕获的Actor仍可通过随后角色命令的版本／角色／recentMFA检查，写入角色变更及审计；当前原会话已经无效。

**反向证据及排除：** `identity/service.go:277–294` Authenticate正确要求未撤销且未过期的session；`httpapi/server.go:245–261`也在解码前取得Actor，所以普通“登出后重新请求”会被拒绝，不能将其报告为失败。缺口限于鉴权已捕获、解码／排队尚未进入命令事务的在途请求，或服务调用继续使用相同捕获Actor；组织锁保证次序，但不补充后发生命令的原会话检查。`platform/commands.go:68–72,79–101`在幂等重放前调用授权，但授权回调本身缺会话检查，重放也不能弥补这一点。原已发生的角色变更会递增授权版本，能阻挡其他旧Actor；单纯登出／会话族撤销不会触发该保护。

现有`identity/interactive.go:14–41`已经提供在同一事务中按原SessionID、渠道、账号／授权版本、未撤销／未过期及数据库近期MFA检查并持有session行锁的机制，可复用到两个角色写入回调。保留原来的角色动作、范围、分类上限、最后管理员与事务审计约束。后续统一批次应覆盖原会话已完成登出／族撤销后相同Actor的拒绝、正常当前会话成功及无额外写入；本次没有执行这些用例。
