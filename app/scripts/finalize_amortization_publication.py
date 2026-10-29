"""Finalize bilingual106 notes after all source-bound evidence passes."""
import json
from pathlib import Path
import subprocess

APP=Path(__file__).resolve().parents[1]
ROOT=APP.parent
E=ROOT/'docs/quality/evidence/cost-amortization'

def read(p):
    return json.loads(p.read_text(encoding='utf-8'))
def save(p,v):
    p.write_text(json.dumps(v,indent=2,ensure_ascii=False)+'\n',encoding='utf-8')
def prepend(name,text):
    p=ROOT/name
    old=p.read_text(encoding='utf-8')
    if text.splitlines()[0] not in old:
        title,rest=old.split('\n',1)
        p.write_text(title+'\n\n'+text.rstrip()+'\n\n'+rest.lstrip('\n'),encoding='utf-8')

def main():
    backend=read(E.parent/'cost-amortization-backend/result.json')
    assert backend['passed'] and read(E/'final-source-check.json')['passed']
    assert read(E/'client-checks.json')['passed'] and read(E/'media.json')['passed']
    tops,records=backend['discovered_top_level_tests'],len(backend['checks'])
    cached=read(E/'android-test-scope.json')['gradle_cached']
    zh='增量106：已接通四端人工费用摊销。明确选择月份均分或按日分配、服务期间和依据，预览逐月项目／公司份额，经最近MFA及口径确认后发布不可变版本；支持停用、完整历史、独立报表、固定导出和冲销沿用原月份汇率。修订或停用重列当前计划视图，原费用／现金事实和已完成导出保留。实际公司制度、跨币种核销、手续费／汇差和完整旅程仍开放；23个工作包（14 P0／9 P1）、5项排除不变。'
    en='Increment 106 adds human-managed amortization across Web, WeChat, Douyin and Android. Choose monthly-equal or daily-weighted service periods, preview project/company shares, complete recent MFA and acknowledge the report basis before publishing an immutable version. Stopping, history, independent reports, frozen exports and original-month FX reversal are supported. Replacing or stopping restates the current schedule view; original expense/cash facts and completed exports remain fixed. Company policy adoption, cross-currency settlement, exchange fees/differences and full workflows remain open; 23 packages remain active, 5 excluded.'
    validation=f'当前源码：后端{tops}个顶层／{records}条结果，Web172、共享移动273、运行时55；微信／抖音各20页、Android开发APK及18+4项静态检查、两个Linux交叉编译。26条JVM按实际'+('缓存结果' if cached else '本轮执行')+'记录。修复移动编译目标对BigInt运算的转换问题后，8项客户端命令重新执行，并新增旧目标编译回归。新增8 PNG／2静音MP4／2 GIF；原199份媒体字节保留。'
    validation_en=f'Checks: backend {tops} top-level tests / {records} results, Web 172, mobile 273, runtime 55; WeChat and Douyin builds with 20 pages each, a development APK with 18 + 4 static checks, and 2 Linux cross-builds. The 26 JVM results retain their actual '+('cached' if cached else 'new execution')+' status. All 8 client commands were rerun after fixing BigInt operator lowering; a regression also executes the compiled legacy-target validator. New media: 8 screenshots, 2 silent MP4s and 2 GIFs; 199 earlier media retain exact bytes. No device, live cloud/model or production acceptance is claimed.'
    for name,text,links,visual in [
      ('README.md',zh+'\n\n'+en+'\n\n'+validation+'\n\n'+validation_en,'[设计与使用 / Design and usage](docs/development/106-cost-amortization.md) · [验证 / Verification](docs/quality/106-cost-amortization-verification.md) · [全部待办 / All remaining work](docs/delivery/03-goal-gap-assessment.md)','![实际费用摊销 / Actual amortization](docs/media/gifs/cost-amortization-web.gif)'),
      ('docs/README.zh-CN.md',zh+'\n\n'+validation,'[完整操作与接口](development/106-cost-amortization.md) · [当前验证](quality/106-cost-amortization-verification.md) · [Web录屏](media/videos/cost-amortization-web.mp4) · [Taro录屏](media/videos/cost-amortization-mobile.mp4)','![移动端费用摊销](media/gifs/cost-amortization-mobile.gif)'),
      ('docs/README.en.md',en+'\n\n'+validation_en,'[Design and human workflow](development/106-cost-amortization.md) · [Verification](quality/106-cost-amortization-verification.md) · [Web MP4](media/videos/cost-amortization-web.mp4) · [Taro MP4](media/videos/cost-amortization-mobile.mp4)','![Actual report](media/screenshots/cost-amortization-web-report.png)')]:
        prepend(name,text+'\n\n'+links+'\n\n'+visual)
    prepend('docs/delivery/03-goal-gap-assessment.md',zh+'\n\n[实现与操作](../development/106-cost-amortization.md) · [本轮验证](../quality/106-cost-amortization-verification.md)。下方保留全部23项稳定编号及5项移出记录，本轮未将完整GAP-03标记完成。')
    prepend('docs/delivery/02-implementation-ledger.md','增量106：迁移83、人工费用摊销计划、逐原份额累计舍入、预览／MFA发布／停用、不可变历史、独立报表、固定导出和冲销原月份折算已实现。完整GAP-03及其余22项继续开放。见[开发与使用](../development/106-cost-amortization.md)和[验证](../quality/106-cost-amortization-verification.md)。')
    prepend('docs/CHANGELOG.md','## 增量106 · 费用摊销 / Versioned cost amortization · 2026-10-29\n\n- '+zh+'\n- '+en+'\n- '+validation+'\n- [完整操作与方案](development/106-cost-amortization.md) · [验证](quality/106-cost-amortization-verification.md)。')
    note=f"""# 增量106：费用摊销验证 / Cost amortization verification

{validation}

| 编号 | 范围 | 证据及结论 |
| --- | --- | --- |
| V-01 | 完整后端 | {tops}个顶层／{records}条结果，当前Go／SQL／嵌入合约前后一致。见[完整批次](evidence/cost-amortization-backend/result.json)及[私有日志归档摘要](evidence/cost-amortization-backend/raw-log-archive.json)。 |
| V-02 | 算法与事务 | 闰日、单日、120月、原币精度和逐份额守恒；权限、MFA重放、审计回滚、并发发布、不可变／延迟约束、历史／停用、范围／CSV及冲销报价日。 |
| V-03 | 四端 | Web172／移动273／运行时55；微信／抖音各20页。[8项重新执行](evidence/cost-amortization/client-checks.json)保存命令时间、日志摘要与最终输入。回归覆盖旧目标编译后的BigInt校验，最终Web与H5均重新录制。 |
| V-04 | 浏览器 | [Web](evidence/cost-amortization-web/result.json)及[Taro H5](evidence/cost-amortization-mobile/result.json)操作真实原生PG／API，使用合成数据，覆盖预览、MFA发布、历史与独立报表；自建页面、上下文、服务和临时库关闭。 |
| V-05 | 媒体 | 8 PNG／2 MP4／2 GIF实际像素，登录／MFA输入结束后录制，无音轨。[编码与采集](evidence/cost-amortization/media.json)及[视觉检查](evidence/cost-amortization/visual-review.json)。原199份按[保留记录](evidence/cost-amortization/reused-evidence.json)核验，合计211份。 |
| V-06 | Android | [开发APK](evidence/cost-amortization/build.json)、[18项检查](evidence/cost-amortization/apk-inspection.json)、[4项功能内容](evidence/cost-amortization/android-feature-content.json)；未执行设备。26条JVM为{'缓存结果' if cached else '本轮执行'}，见[实际任务状态](evidence/cost-amortization/android-test-scope.json)。 |
| V-07 | Linux | 两个ELF64 amd64目标[交叉编译](evidence/cost-amortization/linux-build.json)，未执行、部署；不替代105原生隔离／真实模型验收。 |
| V-08 | 开发环境 | 正常升级schema83，保持历史／使用记录，仅更新已核验归属的API、通知与报表Worker。[更新](evidence/cost-amortization/development-update.json)及[只读核对](evidence/cost-amortization/development-project-resources.json)；临时库0，外部集成未启用；此前超时残留按[名称、OID、创建时间及空闲连接归属核对后清理](evidence/cost-amortization/attempts/07-resource-cleanup/cleanup-result.json)，未使用强制断连。 |
| V-09 | 依赖与范围 | 锁未变，沿用103审计70项（6 high／57 moderate／7 low），不声明新扫描。23项工作包、8项draft Skill、5项排除保持；完整GAP-03未完成。 |

初次类型检查发现共享测试依赖清单缺项；首次服务冷启动超出45秒窗口，[诊断](evidence/cost-amortization/attempts/01-build-input-and-startup/status.json)保留。第二次浏览器在文本定位阶段停止并正确清理，见[原结果](evidence/cost-amortization/attempts/02-text-locator/result.json)。修正定位、显式加载详情样式后重新检查。数据库空服务日期补强后的全量回归使用新源码执行，[早期批次](evidence/cost-amortization/attempts/04-before-date-binding/batch-progress.json)单独保留。随后一批20个测试有19个通过，TestAgentFileText在结束后的临时库删除阶段超时；[失败批次与原日志摘要](evidence/cost-amortization/attempts/05-fixture-cleanup-timeout/status.json)保留，不混作最终通过。随后H5真实页面暴露BigInt幂运算被旧目标转换为Math.pow的兼容问题；[失败页面与中止产物](evidence/cost-amortization/attempts/06-bigint-build-compatibility/status.json)保留。改为整数乘法，补旧目标编译回归后，8项客户端命令与两套实际页面重新执行。最终结果来自相同源码的实际执行及零残留检查。

{validation_en}

不做真机、模拟器、网络性能／实网120GB、历史迁移／容灾或AI费用控制。当前能力是可重列的经营费用分期视图；公司制度、跨币种核销、手续费／汇差、完整四端旅程和外部接入继续按[23项清单](../delivery/03-goal-gap-assessment.md)推进。
"""
    (ROOT/'docs/quality/106-cost-amortization-verification.md').write_text(note,encoding='utf-8')
    agreement=ROOT/'AGENTS.md'
    rule='- Increment106 adds explicit human cost-amortization plans, immutable history and an independent restatable report across four clients. Source expense/cash facts and completed exports stay fixed. Keep company policy adoption, cross-currency settlement, fees/differences and full GAP-03/18 acceptance open. See '+chr(96)+'docs/development/106-cost-amortization.md'+chr(96)+'.'
    if rule not in agreement.read_text(encoding='utf-8'):
        with agreement.open('a',encoding='utf-8') as stream:stream.write('\n'+rule+'\n')
    path=ROOT/'docs/quality/current-functional-baseline.json'
    baseline=read(path)
    baseline.update(increment=106,evidence_group='cost-amortization',verification_script='verify_amortization_sources.py',
      backend_evidence_group='cost-amortization-backend',backend_top_level_tests=tops,backend_result_records=records,
      runtime_evidence_group='cost-amortization',runtime_tests=55,web_evidence_group='cost-amortization',web_tests=172,mobile_tests=273,
      development_verification_script='verify_amortization_development.py',interaction_kind='versioned_cost_amortization',
      interaction_sources='amortization_sources.py',retained_evidence_record='docs/quality/evidence/cost-amortization/reused-evidence.json',
      new_browser_capture=True,new_backend_test_execution=True,full_goal_complete=False,production_ready=False,
      planning_scope_verification_record='docs/quality/evidence/cost-amortization/scope-check.json',
      planning_publication_record='docs/quality/evidence/cost-amortization/repository-check.json')
    changed=subprocess.check_output(['git','diff','--name-only','-z'],cwd=ROOT).decode().split('\0')
    added=subprocess.check_output(['git','ls-files','--others','--exclude-standard','-z'],cwd=ROOT).decode().split('\0')
    kinds={'.md','.py','.go','.sql','.ts','.tsx','.mjs','.cjs','.js','.json','.toml','.yaml','.yml','.xml','.css','.scss','.html','.gradle','.properties','.txt','.mod','.sum','.kt','.kts','.java','.sh','.ps1','.service','.example'}
    baseline['publication_files']=sorted(set(baseline['publication_files']+[v for v in changed+added if v and (ROOT/v).is_file() and Path(v).suffix.lower() in kinds]))
    save(path,baseline)
    print(json.dumps({'passed':True,'increment':106,'backend_top_level':tops,'backend_records':records,'remaining':23,'full_goal_complete':False}))

if __name__=='__main__':
    main()
