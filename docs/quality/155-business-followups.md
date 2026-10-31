# 155 · 申请与交付跟进核验 / Followup verification

本轮采用集中验证。原生场景通过真实认证HTTP接口与独立原生PostgreSQL执行，文件依托已有受控合成对象适配器；没有真实OSS、R2、模型、TG或平台调用。

| 编号 | 核验范围 | 当前证据与界限 |
| --- | --- | --- |
| 1 | 申请草稿、送审、退回、重送 | `native-focused4.json`中的真实普通工作流回执及首页变化 |
| 2 | 交付草稿、提交、修改重提、独立验收 | 同次原生检查，固定修订及普通提交／决定接口；不以读取代表完成 |
| 3 | 数量、权限与会话 | 完整数量／前5条、另一负责人隔离、保留读取但撤销验收权限、分类撤权、旧授权版本／执行身份／撤销会话 |
| 4 | 原生结果 | 11个子场景＋1条套件汇总，共12条通过记录；使用当前源绑定，未执行完整后端回归 |
| 5 | 客户端 | Web4项／共享移动4项协议场景、工作区类型及Web／微信／抖音本地构建；不等于正式平台运行 |
| 6 | 实际Web | 本人退回申请入口、正确项目的准确交付入口、普通独立验收后消除、错误父项目防混淆，共4项 |
| 7 | 媒体 | 3张实际截图、无声实际像素MP4、六秒截图导览GIF；登录与MFA秘密不进入录制 |
| 8 | 保留证明 | 完整后端121／完整客户端136、运行器151的58项、rc.8／APK154固定业务153，均未改称本轮执行 |

原生当前输入包括全部Go／SQL／内嵌API及检查入口；客户端每步保存源列表与前后摘要；实际Web另保存捕获时的源码清单、辅助脚本、构建前后摘要及清理记录。[最终核验](evidence/business-followups/final-source-check.json)逐一核对其当前字节及八个旧冻结引用。

## 失败与修正 / Failures and corrections

1. `native-focused.json`：测试审批人缺少关联资料读取；只移除单个角色仍有另一角色提供验收权限；强行令已有未完成业务的项目进入结项被数据库约束拒绝。补足合法测试角色，实际移除所有提供该验收动作的角色动作并验证普通读取仍可用；不关闭约束来制造结项场景。
2. `typecheck-1.json`：Web采用显式文件清单，新共享协议及用例未纳入。补齐消费者清单后重新检查。
3. `native-focused2.json`：新增浏览器夹具的项目成员参数类型不匹配，编译失败，没有执行场景。
4. `native-focused3.json`：最后的渠道登录测试取用了尚未完成MFA的新登录结果，返回401。保留该失败，改用已完成MFA的真实会话检查旧版本、执行身份和撤销边界；不声称四个真实平台登录已经验收。
5. 早期客户端检查通过只对应当时输入；修正真实项目入口和详情状态隔离后重新构建，最终使用最新精确源记录。旧通过记录不冒充最终源码的验证。

Web导航必须到已有项目交付页签，不能仅凭共享路由字符串用例推断页面存在。本轮补齐真实深链消费、父项目校验和选中对象变化时的状态重置，随后由实际浏览器操作核对。

English: Eleven substantive native cases plus the suite summary pass for the final backend inputs. Actual browser evidence uses a separate short seed branch and four Web checks, including a real ordinary independent acceptance. Retained broad regression, runtime and APK evidence remain explicitly historical. Failed attempts are preserved rather than relabelled.

扩大业务回归、额外移动UI和新Android APK按加速要求延期。八项Skill仍draft。正式签名、公司完整旅程、真实服务器／PG／OSS＋R2／Codex／TG及平台验收仍开放；用户说明资料后续提供。真机、网络性能／实网120GB专项、历史迁移／容灾、AI费用控制继续移出。[完整剩余表](../delivery/05-functional-closure-155.md)。
