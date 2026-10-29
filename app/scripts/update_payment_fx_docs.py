"""Publish bilingual increment116 guides from actual completed evidence."""
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/payment-fx'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def prepend(name, body):
    path = ROOT / name
    raw = path.read_bytes()
    assert '增量116'.encode() not in raw[:500] and b'Increment 116' not in raw[:500], name
    first, rest = raw.split(b'\n', 1)
    newline = b'\r\n' if first.endswith(b'\r') else b'\n'
    path.write_bytes(first + b'\n' + newline + body.strip().encode().replace(b'\n', newline) + newline + newline + rest.lstrip(b'\r\n'))


def main():
    baseline_path = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(baseline_path)
    assert baseline['increment'] == 115
    backend = read(E.parent / 'payment_fx-backend/result.json')
    proof, web, mobile, runtime, media, jvm, upgrade = [read(E / name) for name in ('final-source-check.json', 'web-build.json', 'mobile-build.json', 'runtime-build.json', 'media.json', 'android-test-scope.json', 'schema-upgrade.json')]
    assert all(v['passed'] for v in (backend, proof, web, mobile, media, jvm, upgrade)) and runtime['status'] == 'passed'
    top, records, nw, nm, nr = backend['discovered_top_level_tests'], len(backend['checks']), web['tests']['numPassedTests'], mobile['tests']['pass'], runtime['tests']['pass']
    retained = len(read(E / 'reused-evidence.json')['media'])
    zh = '增量116已接通四端实际付款双币核销：人工固定费用原币金额、实际汇率方向及凭证来源，以付款币核对现金，以费用原币核对费用和员工垫付。部分释放、退款及全额冲销保留两套金额和对齐项目份额，重新加锁核对当前余额，最后一笔处理准确尾差。原始费用／现金事实、历史报价及完成导出保持不变。'
    en = 'Increment 116 connects actual cross-currency payments to incurred costs across Web, WeChat, Douyin and Android. A human fixes the cost amount, actual payment-units-per-cost-unit rate and evidence source. Cash balances use payment units; costs and employee claims use cost units. Partial releases, refunds and full reversals preserve both axes and aligned project shares, recheck locked current balances, and consume the exact final remainder. Original expense/cash facts, quotation history and completed exports stay fixed.'
    usage = '使用：付款与结算 → 已登记原付款 → 分配费用结算 → 选择准确费用（员工收款先选择批准垫付行）→ 输入付款币本金、费用原币金额、实际汇率及凭证来源 → 核对双币并完成MFA → 确认。释放或退款引用原正向分配，核对当前双币余额和显示的回转金额；未知结果先刷新流水。'
    usage_en = 'Usage: open a recorded outgoing payment, choose 分配费用结算 and an actual confirmed cost (for an employee, first select the approved advance line). Enter payment principal, cost units, actual rate and evidence source, acknowledge both axes and confirm with recent MFA. For releases/refunds, select the original positive allocation and review the current dual balances and inverse amount. Refresh the ledger before retrying an uncertain result.'
    boundary = '仍23项工作包（14 P0／9 P1）、5项移出、8项draft Skill。公司财务制度采纳、手续费／汇兑差额、与原付款异币的实收退款及完整GAP-03／18旅程继续开放。实际结算汇率不会自动选用报告估值汇率。仅AI费用控制移出；秘书、TG、授权、审计、隔离及技术限制保留。'
    boundary_en = 'All 23 packages (14 P0/9 P1), five exclusions and eight draft Skills remain. Company accounting policy adoption, fees/exchange differences, refunds received in another currency and full GAP-03/18 journeys remain open. Actual settlements never automatically select a reporting FX book. Only AI fee controls are excluded; secretary, TG, authorization, audit, isolation and technical limits remain required.'
    cached_zh = '来自Gradle缓存' if jvm['gradle_cached'] else '本轮实际执行'
    cached_en = 'from Gradle cache' if jvm['gradle_cached'] else 'executed now'
    validation = f'当前源码：后端{top}顶层／{records}条结果，Web{nw}、共享移动{nm}、运行时{nr}；八项客户端命令、双小程序各20页。Web和Taro H5各6组真实页面／API检查；6 PNG、2静音MP4、2截图导览GIF，{retained}份旧媒体保留原字节。当前开发APK通过18＋3静态检查，26条JVM{cached_zh}。另以已发布115的真实现金用例验证schema88→89正常升级，14张原事实表内容不变；开发服务已按准确源码重载，独立耐久PG关闭。两个Linux目标仅交叉构建，不代表真实云／模型／TG、正式平台或生产验收。'
    validation_en = f'Current source: {top} backend top-level tests/{records} result records; Web {nw}, shared mobile {nm}, runtime {nr}. Eight client commands pass and both miniapps retain 20 routes. Web and Taro H5 each pass 6 actual UI/API groups. New media: 6 PNGs, 2 silent MP4s and 2 screenshot-tour GIFs; {retained} older media retain their bytes. A current development APK passes 18+3 static checks; 26 JVM results are {cached_en}. A separate published 115 cash journey verifies the normal 88→89 upgrade with 14 original fact tables unchanged. Development services reload to exact current sources and the owned durable test PG closes. Two Linux targets only cross-build; live cloud/model/TG, formal platform and production acceptance remain open.'
    for name, prefix, lang in [('README.md','docs/','both'),('docs/README.md','','both'),('docs/README.zh-CN.md','','cn'),('docs/README.en.md','','en')]:
        paragraphs = [zh,en,usage,usage_en,boundary,boundary_en,validation,validation_en] if lang=='both' else ([zh,usage,boundary,validation] if lang=='cn' else [en,usage_en,boundary_en,validation_en])
        links = f'[方案与使用 / Design]({prefix}development/116-cross-currency-settlement.md) · [验证 / Verification]({prefix}quality/116-cross-currency-settlement-verification.md) · [截图／录屏／GIF / Media]({prefix}media/README.md) · [全部未完成项 / Remaining]({prefix}delivery/03-goal-gap-assessment.md)'
        prepend(name,'\n\n'.join(paragraphs+[links]))
    intro = '增量116：GAP-03／18接通四端受控实际付款双币核销、费用及批准员工垫付原币余额、部分回转与双币项目份额。公司财务口径、手续费／汇差、异币实收退款及完整工作包继续开放；23 active（14 P0／9 P1）、5 excluded不变。见[设计](../development/116-cross-currency-settlement.md)与[核验](../quality/116-cross-currency-settlement-verification.md)。'
    for name in ('docs/delivery/02-implementation-ledger.md','docs/delivery/03-goal-gap-assessment.md','docs/delivery/04-current-scope-and-acceptance.md'):
        prepend(name,intro)
    scope_path = ROOT / 'docs/delivery/04-current-scope.json'
    scope = read(scope_path)
    gap_path = ROOT / 'docs/delivery/03-goal-gap-assessment.md'
    raw = gap_path.read_bytes()
    for row in scope['items']:
        if row['id'] != 'GAP-03': continue
        remaining = '继续完成实际公司财务口径选用、手续费／汇兑差额、与原付款异币的实收退款及完整四端财务旅程。增量106提供人工选用摊销；增量116提供固定实际汇率的双币核销及反向分配，制度采纳和完整验收仍开放。'
        assert row['remaining'].encode() in raw
        raw=raw.replace(row['remaining'].encode(),remaining.encode())
        row['remaining']=remaining
        row.setdefault('completed_adaptations',[]).append('CROSS-CURRENCY-SETTLEMENT-116')
        row['completed_detail']+=' 增量116接通四端实际双币本金核销、原币费用／批准垫付结算、对齐项目份额、部分释放／退款／全额冲销、准确尾差与不可变报价。手续费、汇差、异币实收及完整工作包继续开放。'
    gap_path.write_bytes(raw)
    save(scope_path,scope)
    prepend('docs/requirements/05-finance-and-subscriptions.md', '增量116当前约束：受控实际双币核销按人工明确输入的费用原币、付款本金、实际汇率及凭证来源执行，部分释放／退款及全额冲销分别守恒两套余额；费用与批准垫付保持原币。手续费和汇差不自动归类，异币实收退款和公司财务制度采纳仍开放。见[详细约束](../development/116-cross-currency-settlement.md)。下文增量89／106的能力描述保留其原实现范围。')
    path=ROOT/'docs/requirements/05-finance-and-subscriptions.md';raw=path.read_bytes();a='跨币种结算须保存结算汇率/手续费与差额原因，Q-07 未冻结前不开放跨币种分配。'.encode();b='增量116开放人工明确核对本金、费用原币、实际汇率及凭证来源的受控跨币核销；手续费／汇差及异币实收退款不自动处理，Q-07公司财务制度仍待明确采纳。'.encode();assert a in raw;path.write_bytes(raw.replace(a,b,1))
    prepend('docs/CHANGELOG.md','\n\n'.join(['## 增量116 / Increment116 · 实际双币核销 / Reviewed cross-currency settlement',zh,en,validation,validation_en,'路径仍333／操作409；仅5个原模型显式扩展，新增2个报价模型，现731模型。Schema89为原核销账本增加费用原币及报价，旧同币记录第二单位按原额补齐，不改原字段。15组新增API／PG与11组精确金额子用例；13个共享边界用例在Web／移动各执行一次，不算26个不同场景。早期编译、夹具授权、Web编译清单失败及一次中止的客户端检查保留日志摘要，不计最终通过。','[设计与使用](development/116-cross-currency-settlement.md) · [验证](quality/116-cross-currency-settlement-verification.md)']))
    prepend('docs/media/README.md','''增量116 / Increment116：真实认证后的隔离合成双币核销页面。6 PNG、2静音MP4、2截图导览GIF。录屏展示已核对汇率和金额的实际表单，不录制密码／MFA；实际写入和释放由页面检查及流水截图覆盖。GIF为每张2秒的截图导览。

| 端 | 实际汇率 | 窄屏 | 双币流水 | 录屏 | GIF导览 |
|---|---|---|---|---|---|
| Web | [PNG](screenshots/payment-fx-web-quote.png) | [PNG](screenshots/payment-fx-web-narrow.png) | [PNG](screenshots/payment-fx-web-ledger.png) | [MP4](videos/payment-fx-web.mp4) | [GIF](gifs/payment-fx-web.gif) |
| 共享移动 | [PNG](screenshots/payment-fx-mobile-quote.png) | [PNG](screenshots/payment-fx-mobile-narrow.png) | [PNG](screenshots/payment-fx-mobile-ledger.png) | [MP4](videos/payment-fx-mobile.mp4) | [GIF](gifs/payment-fx-mobile.gif) |

[媒体来源](../quality/evidence/payment-fx/media.json) · [验证](../quality/116-cross-currency-settlement-verification.md)。旧媒体保留执行日期及原范围。''')
    verification = f'''# 增量116核验 / Increment116 verification

[业务约束与接口](../development/116-cross-currency-settlement.md) · [全部未完成项](../delivery/03-goal-gap-assessment.md)。

| 编号 | 检查 | 实际结果与边界 |
|---|---|---|
| FX-V01 | 完整后端 | {top}顶层／{records}条结果，12串行批次，全部发现用例执行，无跳过；嵌套结果不等于不同业务场景数。 |
| FX-V02 | 新API与PG | 15组：明确报价／旧协议、双币项目守恒、回转复核、原报价退款、全额冲销、整数币尾差、费用零但现金剩余、零现金项目份额、跨付款费用上限、并发、SQL伪造拒绝、独立退款后的重检、现金及关闭单位、真人MFA幂等、已批准员工垫付。 |
| FX-V03 | 纯金额与客户端 | 精确Go金额11组；Web{nw}、移动{nm}、运行时{nr}。13个新共享边界用例在Web和移动复用，不加倍计业务场景；另有2个移动表单／旧草稿用例。 |
| FX-V04 | 四端构建 | 八项类型／测试／构建命令通过；微信／抖音各20页。Android当前开发APK18＋3静态检查；26条JVM{cached_zh}，保留4份XML，未运行设备或模拟器。 |
| FX-V05 | 真实页面 | Web、Taro H5各6组，实际API／PG，不替换响应：显式合成授权与事实；错误报价阻止写入；正确报价及MFA；部分双币释放；原事实保留与双币流水；无财务权限用户拒绝。基础运行夹具复用已存在文件范围测试，财务授权和数据经实际API创建，不称新增财务Go夹具。 |
| FX-V06 | 升级 | 已发布115的schema88完整真实现金用例先执行，再以当前正常migrator升至89；14张原事实表不变，旧核销／项目份额按原额补齐。此独立旧源码升级检查不计当前后端数量。 |
| FX-V07 | 媒体 | 6 PNG、2静音MP4及2截图导览GIF；认证信息不入素材，MFA输入不录制。{retained}份旧媒体保持原字节，不包装成新验收。 |
| FX-V08 | 服务与资源 | 原生开发PG88→89，准确源码API／两项worker重载，旧AI费用资料及原现金／费用表保持；独立耐久测试PG关闭，0临时库，浏览器／临时端口关闭。外部集成保持关闭。 |
| FX-V09 | 合约与Linux | 原333路径／409操作不变，5模型显式扩展＋2新模型，731模型；两个Linux目标交叉构建，不是目标机执行。 |
| FX-V10 | 当前范围 | 23 active／5 excluded，八项Skill仍draft；公司财务制度、费用／汇差及完整旅程保留GAP-03／18。适用性分类不执行业务案例，完整目标未完成。 |

首轮新Go测试有两处测试代码编译问题，随后员工报销夹具缺少独立读取授权；修正后另行运行最终定向和完整回归。Web首次类型检查遗漏共享文件编译清单；一次客户端检查在完善回执币种绑定前主动中止。失败／中止日志保留私有文件摘要，不计本轮最终通过；浏览器若有早期失败，另列attempt记录，不能替代最终源码验证。

首次页面准备因给自己授权后原会话正确失效而停止；改用独立管理员授权并重新登录。共享移动页面随后暴露本地金额校验被通用提示隐藏的问题，已修正同步财务输入边界，并重新执行受影响客户端、Android及两端页面检查。修正前通过的客户端／APK／Web证据、原源码、原素材与日志单独保留，不计当前源码通过。独立升级首次实际迁移通过，但收集器未拼接Go长日志分段，修正后只重跑该升级检查。

进一步诊断确认Taro把BigInt幂改写为`Math.pow`，导致实际移动页面在报价校验前发生类型错误；共享计算改用有界BigInt乘积，并检查真实编译H5业务流程与新APK的已知改写模式。此发现说明源码单元测试不能代替实际构建消费者检查，原失败与诊断不计最终通过。

修正前的交互源码归档没有覆盖客户端清单中的另外26个兼容包／验证脚本。补充归档使用未变的已发布Git字节及可由原聚合摘要确认的原脚本字节，要求完整原客户端和Android源码摘要同时一致；保留最初归档原字节，不计新测试执行。

最终仓库发布检查另发现Web付款页面末尾有一个额外空行。修正前通过的客户端、两端页面、媒体及失败发布记录连同准确源码和私有日志分别归档；移除该2字节空行后，重新执行八项客户端命令及Web／Taro H5两端页面检查和媒体生成。后端及Android输入的准确摘要未改变，保留其原执行时间与缓存状态，不称重新执行。

该EOF归档另漏列了浏览器清单特有的`build_openapi.py`和`project_concern_picker.mjs`，使首次最终来源收集器在生成报告前停止。已从独立绑定的导航修正前源码归档补齐，要求原客户端及两个原浏览器完整摘要同时一致；原归档和收集器保持原字节，此补全不计新业务执行。

这次移动页面复查已通过前4组，但脚本在应用核对回执并自动返回详情期间再次强制跳转，导致流水读取超时，失败截图显示加载中的详情与两个活动页面。修正仅让脚本等待真实详情导航和流水区就绪；未修改应用、断言、超时或替换响应。原失败及同一脚本版本下已通过的Web证据分别归档，随后以修正后的准确脚本重新检查两端；不重复计作客户端、后端或Android测试。

| 证据 | 链接 |
|---|---|
| 准确源码 | [最终检查](evidence/payment-fx/final-source-check.json) |
| 后端 | [全量](evidence/payment_fx-backend/result.json)、[批次](evidence/payment_fx-backend/batch-progress.json)、[原日志摘要](evidence/payment_fx-backend/raw-log-archive.json) |
| 客户端与页面 | [八项命令](evidence/payment-fx/client-checks.json)、[Web](evidence/payment-fx-web/result.json)、[Taro H5](evidence/payment-fx-mobile/result.json) |
| Android | [构建](evidence/payment-fx/build.json)、[18项静态检查](evidence/payment-fx/apk-inspection.json)、[双币内容](evidence/payment-fx/android-feature-content.json)、[JVM范围](evidence/payment-fx/android-test-scope.json) |
| 升级／开发 | [旧版实际升级](evidence/payment-fx/schema-upgrade.json)、[开发更新](evidence/payment-fx/development-update.json)、[只读检查](evidence/payment-fx/development-project-resources.json) |
| 媒体 | [来源与编码](evidence/payment-fx/media.json)、[目视检查](evidence/payment-fx/visual-review.json)、[旧媒体](evidence/payment-fx/reused-evidence.json) |
| Linux／关闭 | [交叉构建](evidence/payment-fx/linux-build.json)、[原生PG关闭](evidence/payment-fx/native-cluster-closed.json) |
| 初期尝试 | [日志摘要](evidence/payment-fx/attempts/early-checks.json) |
| 追加尝试与修复 | [会话失效](evidence/payment-fx/attempts/01-browser-session-header/diagnosis.json)、[移动提示失败](evidence/payment-fx/attempts/02-mobile-validation-message/mobile/result.json)、[修正前通过范围](evidence/payment-fx/attempts/03-before-mobile-validation-fix/preservation.json)、[升级收集](evidence/payment-fx/attempts/upgrade-output-chunks.json) |
| 移动构建诊断 | [实际通用提示](evidence/payment-fx/attempts/04-mobile-notice-diagnostics/mobile/result.json)、[BigInt改写根因](evidence/payment-fx/attempts/05-taro-bigint-lowering/diagnosis.json) |
| 原源码归档补全 | [26个未变输入与完整原摘要](evidence/payment-fx/attempts/03-before-mobile-validation-fix/source-archive-supplement.json) |
| 发布格式修正 | [原通过结果、失败发布记录、源码与媒体](evidence/payment-fx/attempts/06-web-eof-whitespace/preservation.json) |
| EOF归档补全 | [两个原浏览器输入及原摘要](evidence/payment-fx/attempts/06-web-eof-whitespace/source-archive-supplement.json) |
| 移动导航检查 | [原失败与已通过Web的准确来源](evidence/payment-fx/attempts/07-mobile-navigation-race/navigation-preservation.json) |

These checks establish the bounded, explicitly reviewed settlement workflow. Live providers, formal releases, real Codex/TG and production acceptance remain open. Prior and cached evidence retains its original execution scope.
'''
    (ROOT/'docs/quality/116-cross-currency-settlement-verification.md').write_text(verification,encoding='utf-8')
    (E/'README.md').write_text('# 增量116证据 / Increment116 evidence\n\n准确源码、完整本地回归、真实页面与像素、旧版已记账升级、开发服务重载和资源关闭分别记录。旧媒体及JVM缓存保留原范围，失败／中止尝试不计最终通过。凭据、数据库、APK及原始帧保持私有。\n\n[方案](../../../development/116-cross-currency-settlement.md) · [核验](../../116-cross-currency-settlement-verification.md)。\n',encoding='utf-8')
    path=ROOT/'AGENTS.md';raw=path.read_bytes();raw+=b'\r\n'+b'- Increment116 adds reviewed actual cross-currency cost/claim settlement across four clients, schema89 and dual-unit inverses. Preserve original facts, explicit quotation evidence, human MFA and independent balances. Company policy, fees/exchange differences, other-currency refunds and full GAP-03/18 remain open. See `docs/development/116-cross-currency-settlement.md`.\r\n';path.write_bytes(raw)
    baseline.update(increment=116,evidence_group='payment-fx',mobile_tests=nm,verification_script='verify_payment_fx_sources.py',backend_evidence_group='payment_fx-backend',backend_top_level_tests=top,backend_result_records=records,web_evidence_group='payment-fx',runtime_evidence_group='payment-fx',web_tests=nw,runtime_tests=nr,development_verification_script='verify_payment_fx_development.py',interaction_kind='human_cross_currency_settlement',interaction_sources='payment_fx_sources.py',retained_evidence_record='docs/quality/evidence/payment-fx/reused-evidence.json',planning_scope_verification_record='docs/quality/evidence/payment-fx/scope-check.json',planning_publication_record='docs/quality/evidence/payment-fx/repository-check.json')
    changed=subprocess.check_output(['git','diff','--name-only','-z'],cwd=ROOT).decode().split('\0')
    added=subprocess.check_output(['git','ls-files','--others','--exclude-standard','-z'],cwd=ROOT).decode().split('\0')
    future=['docs/quality/evidence/payment-fx/'+name for name in ('final-source-check.json','scope-check.json','acceptance-applicability-check.json')]
    names=set(baseline['publication_files']+future+[name for name in changed+added if name and (ROOT/name).is_file()])
    baseline['publication_files']=sorted(name for name in names if Path(name).suffix.lower() not in {'.png','.gif','.mp4','.webm','.log'} and name!='docs/quality/evidence/payment-fx/repository-check.json')
    save(baseline_path,baseline)
    print(json.dumps({'passed':True,'increment':116,'backend_top_level':top,'backend_records':records,'active':23,'excluded':5}))


if __name__=='__main__':
    main()
