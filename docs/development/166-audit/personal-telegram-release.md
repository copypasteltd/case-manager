# 个人 Telegram 通知与受控原生发布兼容性审查

基线：增量165提交`ddf6a70cc2de5fba5d01f64d581c4a228052c1d7`。审查增量153的个人Telegram通知进程与增量119引入、当前仍使用的受控发布工程。只读应用源码、公开服务模板及部署指南；未运行测试、构建、systemd、真实网络或消息，未读取私有配置／凭据，未更改应用、旧迁移或历史证明。

**Verdict：partial，受控发布接入存在确定缺口。** 个人通知工作器及手工部署模板真实存在，不能裁定为“未实现发送器”。但是旧模板不是119布局的直接可用服务，受控服务清单无法启用它；进一步，当前plain环境文件语法无法表示现有Bot加载器要求的非空JSON配置。因此修复必须同时覆盖服务、配置与发布生命周期，不能只在catalog增加一个名称。

## 1. 功能入口真实存在

- [153部署要求](../153-personal-telegram-notifications.md)，44–58行：继续运行普通站内通知工作器；新增`platform worker-notification-telegram`；明确沿用`telegram-delivery-worker.env`的数据库配置和`CP_TELEGRAM_DELIVERY_BOTS`。API接收配置、双向绑定、员工偏好及真实域名仍需另行设置，不由出站进程制造。
- [CLI分发](../../../app/backend/cmd/platform/main.go)，89–90行：识别该命令并调用[工作器](../../../app/backend/cmd/platform/notification_telegram_worker.go)，15–53行。后者读取普通Worker配置及Bot配置、打开PG、核对完整schema，然后循环调用`notification.Service.DeliverTelegramOnce`。
- [普通Worker配置](../../../app/backend/internal/platform/worker_config.go)，13–31行：必须有`CP_DATABASE_URL`，禁止迁移凭据；production要求`sslmode=verify-full`。[Bot加载器](../../../app/backend/internal/telegram/delivery_config.go)，29–61行：只读`CP_TELEGRAM_DELIVERY_BOTS`，空值／disabled返回空集；非空值必须是1–8个严格JSON对象，逐项验证Bot身份、Token格式和准确HTTPS门户。个人工作器25行对空Bot集合直接拒绝启动。

以上证明命令、运行能力和必需配置都有实现；是否在真实机器安装、是否有有效绑定／同意及实际送达，源码不能证明。

## 2. 手工模板与119发布布局逐项比较

| 边界 | 153手工模板 | 119当前受控路径 | 静态判断 |
|---|---|---|---|
| 服务名称 | `copypaste-notification-telegram-worker.service` | `catalog.unit(name)`只生成`copypaste-release-<固定名称>.service` | 当前catalog没有个人TG名称，模板不进入受管集合 |
| 二进制 | `/opt/copypaste/bin/platform worker-notification-telegram` | `/opt/copypaste/current/bin/platform <固定命令>` | 冻结包暂存到`releases/r-…/bin/platform`，119只维护`current`链接，不创建旧`bin/platform`别名 |
| 工作目录 | `/opt/copypaste/app` | `/opt/copypaste/current/runtime/app` | 没有发布器创建旧`app`兼容目录的已读路径，按119干净布局不能假定该目录存在 |
| 环境文件 | `/etc/copypaste/telegram-delivery-worker.env` | `/etc/copypaste/active/<catalog固定env>`；现有群回执为`telegram-delivery.env` | 名称、版本化路径不同；controller只切换`active`，不创建旧环境文件 |
| Linux身份 | `copypaste-worker`用户及同名组 | 固定服务分别使用`copypaste-notification`、`copypaste-delivery`等身份 | 管理员只按119固定清单创建账号，不能假定旧通用账号存在；模板存在不等于权限已经具备 |
| 维护／迁移／切换／恢复 | 独立于受控清单的旧unit | 仅对当前／目标固定unit集合执行stop、start、healthy及恢复 | 额外手工进程不会自动被发布器接管，不能把它视为已参与受控停启 |

已读依据：[153原生模板](../../../app/deploy/native/copypaste-notification-telegram-worker.service)，2–23行；[既有群回执手工模板](../../../app/deploy/native/copypaste-telegram-delivery-worker.service)，2–23行，使用相同旧目录／环境／通用用户，证明153对手工路径的复用是有来源的。[catalog](../../../app/deploy/release/catalog.py)，2–19、22–74行；[controller](../../../app/deploy/release/controller.py)，278–315、411–415、424–465行；[生产构建](../../../app/scripts/build_production_release.py)，84–114行只按catalog渲染服务单元。[运行手册](../../operations/03-native-release-runbook.md)，9–25行明确`current`／`active`布局，并要求盘点停用旧模板和手工进程，不允许带未受管旧Worker升级数据库。

若管理员额外创建旧目录／兼容链接、账号和环境文件，手工unit可能运行。真实机器资料未提供，因此不能断言它一定运行失败；但这种额外布置仍没有将服务纳入119控制器的准确单元核验、配置摘要及停启／恢复流程。

## 3. 可复核的确定阻塞路径

以下是源码条件推导，没有执行配置读取或故障注入。

| 编号 | 输入／操作 | 真实条件链 | 结果 |
|---|---|---|---|
| PT-R01 | 在`deployment.json.units`加入`notification-telegram`，或加入旧unit文件名以启用个人通知 | `controller.configuration:228–230`要求`set(units) <= SERVICES.keys()`；`catalog:2–19`无该命令／键 | `Invalid enabled-service set`，不存在受控启用入口 |
| PT-R02 | 保持有效units，只向版本化配置目录加入153要求的`telegram-delivery-worker.env` | `controller:235–253`从固定catalog推导准确required集合，并要求与目录文件集合相等 | 该额外文件不能被接纳；同名手工文件放在根目录则在受控配置指纹之外 |
| PT-R03 | 在现有合法`telegram-delivery.env`里填写153文档55行要求的非空Bot JSON数组 | `controller.parse_env:105–114`禁止值含双引号、单引号和反斜线；合法非空对象数组必须包含JSON引号；加载器`delivery_config.go:36–61`只接受原始JSON，没有Base64或文件入口 | 配置解析先拒绝，尚未启动工作器。此问题也影响已有受控`telegram-delivery`，不是个人通知特有 |
| PT-R04 | 用`disabled`／空值绕开JSON限制；或仅增加catalog而保持现状 | `LoadDeliveryBots`返回零项，`notification_telegram_worker.go:24–25`拒绝启动；加catalog也不会改变`parse_env` | 不能获得可用发送配置，catalog单点修改不闭环 |
| PT-R05 | 直接安装旧手工unit并继续使用119发布 | `NativeEffects.preflight:157–161`只核对声明的`copypaste-release-*`单元；`execute:426–465`只停止／启动固定当前及目标服务；`switch:411–415`只换应用和配置链接 | 未受管单元不自动进入生命周期；即使增加路径别名，也不能据此宣称受控发布完整 |

PT-R03不依赖Token的真实值是否有效；JSON对象的必要语法已与解析器冲突。它也不证明手工systemd环境文件使用正确引用后必然失败，本审查没有执行systemd解析。放宽Python解析器后仍需保证进程获得的准确JSON与被审阅／固定的配置一致，不能只验证Python接受文本。

## 4. 最小一致修复建议

建议在下一整批源码波采用一条明确的受控接入路线：

1. 在固定catalog加入个人TG服务键，固定`worker-notification-telegram`命令、专用非root身份及生产环境文件；让现有渲染器、打包和控制器统一生成／核验／停启它。继续保留普通notification工作器这一前置依赖，并在部署说明中明确组合；不能只启动出站器就期待产生站内事件。
2. 对结构化Bot秘密采用已有status-bridge的固定凭据文件模式：增加**明确的Bot文件读取入口**及严格解析，systemd通过`LoadCredential`提供，controller允许并固定这个准确文件。启用相应发送服务时才要求此文件；同时适配个人发送和群回执两个消费者。保留手工环境兼容入口时规定唯一来源／冲突拒绝，避免悄悄选择旧Token。不要为承载JSON而无边界放开所有环境文件引用、反斜线或任意文件路径。
3. 更新受控配置白名单／必需文件推导、配置指纹和部署指南，覆盖准确Bot／组织／HTTPS门户对应关系、普通生产PG配置、账号权限、旧手工unit停用与重复发送检查。新格式应明确与旧配置的关系；旧冻结发布包和证明保持原样。
4. 同批保存正反配置用例和服务清单检查：合法新服务与准确凭据、未知服务／多余文件、两个来源冲突、空／非法JSON、Bot／门户不匹配、旧模板未停用，以及切换／恢复时新服务参与清单。源码／用例全部保存后统一检查，本审查不提前运行局部测试。

上述是兼容现有发布架构的建议，不代表必须采用新的监控产品，也不改变个人通知的显式同意、当前权限、固定安全提示、未知发送不自动重发等业务约束。管理员仍需后续提供实际服务器、Bot资料、公司门户和接收绑定，并执行真实部署／平台验收；这些未知项与已确认的源码接入缺口分开保留。
