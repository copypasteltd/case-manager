# TG受控发布 / Controlled Telegram release

增量166补齐固定 `notification-telegram` 服务：命令 `worker-notification-telegram`，独立用户/组 `copypaste-notice-tg`，普通发布路径 `/opt/copypaste/current/bin/platform`，配置版本位于 `/etc/copypaste/configurations/<id>`。与group delivery使用同一固定JSON来源，但systemd分别交付凭据给不同服务用户；共享Bot预算、本人订阅授权、当前普通权限与不确定送达语义保持。

| 启用服务 | 普通环境文件 | root私密配置文件 | systemd固定FILE变量 |
|---|---|---|---|
| api | api.env | telegram-ingress.json | CP_TELEGRAM_INGRESS_FILE |
| telegram | telegram.env | telegram-dispatch.json | CP_TELEGRAM_DISPATCH_BOTS_FILE |
| telegram-delivery | telegram-delivery.env | telegram-delivery.json | CP_TELEGRAM_DELIVERY_BOTS_FILE |
| notification-telegram | notification-telegram.env | telegram-delivery.json | CP_TELEGRAM_DELIVERY_BOTS_FILE |

所有部署都显式提供 `telegram-ingress.json`；TG关闭时内容是空数组 `[]`，不需要实际Bot密钥。开启TG时用Go配置结构要求的1–8个对象：ingress只含bot_id、organization_id、username、webhook_secret；dispatch只含bot_id、organization_id；delivery只含bot_id、organization_id、token、portal_origin。delivery的token前缀须等于bot_id，portal_origin是无路径/查询的HTTPS源。未知字段、重复JSON键、重复身份、错误字段与超过8192字节均拒绝。不要把真实内容复制到本手册或聊天。

固定units用 `LoadCredential` 把所需文件交付到服务的 `%d` 目录，环境中只出现FILE路径。所有JSON与环境文件仍由root保护；发布控制器对完整配置集计算指纹，并随配置指针切换/失败恢复。普通环境文件继续只允许严格plain KEY=value，禁止直接TG JSON或任何自定义TG *_FILE路径；不要用引号、shell替换或放宽解析绕开。

非受控原生开发环境保留历史inline配置兼容；新增 `KEY_FILE` 与 `KEY` 必须二选一，即使inline为空也拒绝两源。文件必须绝对规范路径、普通文件、无最终软链接、1–8192字节；原生Unix要求禁止组/其他用户读写权限，读取前后核对同一文件。Windows测试不证明Linux权限。错误不打印路径、令牌、配置正文或OS异常。

配置 `deployment.json.units` 显式加入需要的服务，同时提供对应独立生产DB身份及上述固定文件。先离线核对完整配置与unit，再按既有原生受控发布步骤安装/激活；不可把真实配置放入构建包。生产启用前核对153历史手工unit `copypaste-notification-telegram-worker.service` 的所有权与运行情况，经运维审阅停用旧自有进程，避免其继续调用 `/opt/copypaste/bin/platform`。当前代码不擅自停止用户服务，也不自动创建真实Bot绑定或本人订阅。

Telegram write-grant通知的 accepted/progress/result 仅含固定状态及内部入口，原请求/模型正文不入群。当前组织授权版本和原run/actor身份在排队/准备时复核；已确认送达历史不改写。组织版本变化后旧待发接受通知suppressed；发出后结果不确定仍只可明确人工对账，不自动重发。人工复核结果披露走既有独立授权分支。

English: enable the fixed notification-telegram release service and provide only its dedicated DB environment and bounded root-private delivery JSON. PID1 copies the required credential to each service identity; no provider token appears in shell syntax, release manifests or logs. Keep api ingress explicitly disabled with [] when unused. Versioned configuration hashes cover these files. Review and retire the old hand-installed unit before production enablement. Local doubles/parser checks do not establish real Bot binding or delivery.
