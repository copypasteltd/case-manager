"""Write current delivery notes from completed execution records, never planned counts."""
from datetime import datetime, timezone
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[2]
E=ROOT/'docs/quality/evidence/finance-fx'
def read(path):return json.loads(path.read_text(encoding='utf-8-sig'))
def write(path,data):path.write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')

def main():
    backend=read(E.parent/'finance-fx-backend/result.json')
    clients=read(E/'client-checks.json');android=read(E/'build.json');jvm=read(E/'android-test-scope.json')
    media=read(E/'media.json');development=read(E/'development-project-resources.json');retained=read(E/'reused-evidence.json')
    browsers=[read(E.parent/('finance-fx-'+v)/'result.json') for v in ['web','mobile']]
    assert all(v['passed'] for v in [backend,clients,android,jvm,media,development,retained,*browsers])
    assert backend['all_discovered_tests_executed'] and backend['source_unchanged_during_tests'] and all(v['Action']=='pass' for v in backend['checks'])
    assert development['migration_count']==81 and development['database']['disposable_test_databases']==0
    top,count=backend['discovered_top_level_tests'],len(backend['checks'])
    client_counts={v['name']:v['test_count'] for v in clients['checks'] if v['test_count'] is not None}
    jvm_scope='Gradle缓存结果，非本轮重新执行' if jvm['gradle_cached'] else '本轮实际执行'
    canonical={kind:len(list((ROOT/'docs/media'/folder).glob('*'+ext))) for kind,folder,ext in [('PNG','screenshots','.png'),('MP4','videos','.mp4'),('GIF','gifs','.gif')]}
    retry_note = ('同源完整批次原有失败或缺失：保留[首次结果](evidence/finance-fx-backend/attempt01/result.json)及原始日志摘要，再独立串行执行明确选定的父用例：' + '、'.join(v['test'] for v in backend['retries']) + '。最终记录保留通过父用例，整体替换重试父用例的全部结果；首次失败仍可核对，未改写为通过。详见[重试来源](evidence/finance-fx-backend/retry-progress.json)。' if backend.get('retries') else '本次当前来源完整批次无需父用例重试。')
    note=f'''# 增量102验证：人工汇率版本与财务折算

日期：2026-10-28—29（Asia/Taipei，跨日完成）。功能、使用方式、计算规则及后续事项见[开发说明](../development/102-finance-fx-valuation.md)。本轮完成GAP-03的报表折算子项；跨币种核销、手续费／汇差、摊销和完整财务旅程仍未完成。当前仍为23项完整工作包开放（14 P0／9 P1），5项按[修订99](../delivery/04-current-scope-and-acceptance.md)移出。

## 实际完成的检查

| 编号 | 范围 | 当前证据 |
|---|---|---|
| V-01 | 原生后端 | {top}个顶层测试、{count}条最终测试结果全部通过，完整发现集与实际执行集一致；Go／SQL／内嵌契约来源摘要相同。见[结果](evidence/finance-fx-backend/result.json)。 |
| V-02 | 财务专项 | 新增10条版本／权限／精确折算／缺口／冲销／并发／分页／CSV旅程及精确大数舍入测试。发生204 HKD、现金90 HKD、已保存预测200 HKD分开核对；预测源覆盖不完整时完整合计为空。 |
| V-03 | 客户端 | Web {client_counts['web-tests']}、移动 {client_counts['mobile-tests']}、运行时 {client_counts['runtime-tests']}项测试通过；类型、Web／运行时及微信／抖音构建通过。两小程序各20页。见[真实命令记录](evidence/finance-fx/client-checks.json)。 |
| V-04 | Android | 当前JS bundle与新开发APK来源绑定，APK内bundle逐字节一致；18项包结构检查及4项新功能内容检查通过。构建没有启动设备或模拟器。见[构建](evidence/finance-fx/build.json)。 |
| V-05 | JVM | 26项结果，{jvm_scope}；原始任务状态、XML和日志摘要保留在[范围记录](evidence/finance-fx/android-test-scope.json)，不提升为真机验收。 |
| V-06 | 实际Web／Taro H5 | 均完成真实MFA登录后的版本选择、缺口查看、人工发布第4版、204→205 HKD折算及固定版本CSV；下载字节与回执SHA-256一致。各3条业务检查通过。 |
| V-07 | 界面与媒体 | 各在390／768／1440宽度检查折算块；审看4张截图，生成2段真实像素无声MP4和2个GIF。当前规范媒体为{canonical['PNG']} PNG＋{canonical['MP4']} MP4＋{canonical['GIF']} GIF。 |
| V-08 | Linux | 当前后端交叉编译成功；没有运行该二进制或部署到服务器。见[编译记录](evidence/finance-fx/linux-build.json)。 |
| V-09 | 本地开发 | 正常追加迁移81，已有历史和用量行保持；API与通知／导出Worker由已有进程归属记录核验后更新。测试库最终0个，真实外部集成保持关闭。 |
| V-10 | 历史保留 | 原181份规范媒体及原审计／JVM记录保持已发布字节，见[复用清单](evidence/finance-fx/reused-evidence.json)。保留的依赖审计仍有80项（13高／60中／7低），不是本轮新审计或已关闭安全门。 |

## 真实操作与来源

| 客户端 | 检查 | 操作截图 | 录屏／GIF |
|---|---|---|---|
| Web | [真实API结果](evidence/finance-fx-web/result.json)、[CSV证明](evidence/finance-fx-web/download-proof.json) | [缺口](../media/screenshots/finance-fx-web-before.png)、[第4版205 HKD](../media/screenshots/finance-fx-web-after.png) | [MP4](../media/videos/finance-fx-web.mp4)、[GIF](../media/gifs/finance-fx-web.gif) |
| 共享Taro H5 | [真实API结果](evidence/finance-fx-mobile/result.json)、[CSV证明](evidence/finance-fx-mobile/download-proof.json) | [缺口](../media/screenshots/finance-fx-mobile-before.png)、[第4版205 HKD](../media/screenshots/finance-fx-mobile-after.png) | [MP4](../media/videos/finance-fx-mobile.mp4)、[GIF](../media/gifs/finance-fx-mobile.gif) |

录屏仅包含登录后的隔离合成业务页面；无密码、MFA秘密、真实报价或公司财务记录。原始帧和CSV临时内容留在忽略目录，发布文件及帧清单有摘要绑定。Web和H5的所有自建页面、上下文、浏览器与5189临时服务均已关闭，夹具数据库已清理；保留日常开发API、数据库和Worker。

## 失败与中断的处理

前期专项探查在旧导出回执仍为排队对象时直接解引用空摘要，导致测试夹具失败。修正为先读取已生成的真实导出回执，再比较原字节；失败探查不计入最终通过证明。原始诊断日志只保存在忽略目录，公开摘要见[诊断记录](evidence/finance-fx/diagnostic-attempts.json)。

首次完整回归的原生进程句柄失效，检查时本地PostgreSQL已停止，批次没有最终成功结果。其原进度、运行源摘要和日志摘要保留在[中断记录](evidence/finance-fx-backend/attempts/native-process-interruption/interruption.json)，不推测停止原因、不把部分结果拼为通过。恢复同一原生开发集群后重跑完整发现集，未重建数据目录、未降低持久性，也未启用虚拟化。其遗留的单个合成夹具在精确核对OID、创建时段、原日志、81项迁移账本、测试种子和零连接后清理，见[清理证明](evidence/finance-fx/interrupted-fixture-cleanup.json)。

最终权限复核发现，发布接口曾用当前员工作为报表所有者检查权限，可能使仅限自己的授权修改公司口径。已改为检查组织资源，实际验证个人、项目及无关资源范围均拒绝。旧串行回归在检测到源码变化后退出，旧进度及日志摘要保存在[权限复核记录](evidence/finance-fx-backend/attempts/organization-scope-hardening/superseded.json)；旧浏览器成功记录也保留在各自尝试目录。最终完整回归以两个独立原生批次并行重跑，页面、导出、媒体及Linux构建重新绑定当前来源。开发API重载曾遇到Windows监听端口释放延迟，立即检查失败后停止；保留[该次记录](evidence/finance-fx/attempts/organization-scope-hardening/development-reload.json)，增加有界等待并重新通过所有自有服务检查。

浏览器第一次探查在页面挂载前判断筛选框状态，随后点击按钮把默认展开的筛选收起。原[失败记录](evidence/finance-fx-web/attempts/filter-mount-timing/result.json)保留，关闭资源后修正脚本等待条件并重新运行Web／Taro H5操作。最终证明绑定当前脚本与实现来源。

共享H5复验曾在浏览器启动前超出480秒构建时限，未将webpack完成消息当作构建成功，原始日志摘要保存在[构建超时记录](evidence/finance-fx-mobile/attempts/native-build-timeout/failure.json)。随后使用同一Taro CLI的直接Node入口，显式持有构建及浏览器进程句柄，限定构建时长并在退出时清理自建进程树；夹具正常清理等待为150秒。最终两端页面和媒体重新采集，绑定更新后的检查脚本。

既有 `TestMiniappBrowserFixture` 首次运行在合成数据库 `t.Cleanup` 的 `DROP DATABASE` 操作处超过120秒，源文件第86行报告 `context deadline exceeded`；保留[脱敏诊断及日志摘要](evidence/finance-fx/backend-cleanup-timeout.json)。该失败不能计入通过。遗留数据库已进入PostgreSQL无效状态，无法读取内部账本；依据精确OID、创建时段、失败时刻对应的服务器DROP语句、原测试来源及零连接状态完成[定点清理](evidence/finance-fx/timeout-fixture-cleanup.json)，未强制断开连接或重启数据库。所有合成测试数据库现已释放。

{retry_note}

发布清单首次把PNG、GIF、MP4和原始Gradle日志列入文本空白检查，保留[原失败结果](evidence/finance-fx/attempts/publication-text-scope/repository-check.json)。已修正本轮清单生成器，只把代码／文档／结构化文本加入空白检查；所有候选文件继续接受敏感内容、路径、大小、链接和产物来源检查，未改写媒体或原始日志来消除报告。

如当前后端结果中另有重试来源，以结果及其原尝试目录逐项记录为准；最终统计只采用同一当前来源、已实际通过的完整测试集合。没有把历史缓存、旧源执行或取消范围当成本轮验收。

## 复现与交付边界

```powershell
python app/scripts/verify_finance_fx_backend.py
python app/scripts/verify_finance_fx_clients.py
python app/scripts/run_finance_fx_browser.py
python app/scripts/run_finance_fx_browser.py --mobile
python app/scripts/build_android.py --google-mirror --evidence-group finance-fx
python app/scripts/build_finance_fx_linux.py
python app/scripts/verify_finance_fx_sources.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_repository_publication.py
```

本地Android构建需显式设置 `CP_MOBILE_BUILD_PROFILE=local`、`CP_MOBILE_API_ORIGIN=http://127.0.0.1:18080`；默认正式构建要求明确HTTPS地址。后端检查需现有原生PostgreSQL及忽略环境文件。已完成证据不会被脚本无提示覆盖，重跑前保留原尝试。四端源码、本地编译和实际Web／H5功能操作不等同于真实平台／生产验收。

**本轮未完成**：实际公司财务口径选用、跨币种核销、汇差／手续费、摊销、完整四端财务旅程，以及[全部剩余23项](../delivery/03-goal-gap-assessment.md)。8项Skill仍为draft，真实Codex、运行隔离、TG、云存储、生产部署及依赖安全处置继续开放。真机、网络性能／实网120GB专项、历史迁移／容灾与AI费用控制继续移出。

English: All{top} current-source backend tests pass with{count} final result records. Web{client_counts['web-tests']}/mobile{client_counts['mobile-tests']}/runtime{client_counts['runtime-tests']} tests, four-client local builds and real authenticated Web/Taro H5 FX publication and pinned CSV journeys pass. Android JVM results are explicitly classified as {'cached' if jvm['gradle_cached'] else 'newly executed'}, without device acceptance. The interrupted native attempt and initial browser/test-fixture failures remain recorded. Existing181 canonical media and retained audit/JVM bytes are preserved. Cross-currency settlement, fees/exchange differences, amortization, full finance acceptance and all23 complete work packages remain open; this is not a production-readiness claim.
'''
    (ROOT/'docs/quality/102-finance-fx-verification.md').write_text(note,encoding='utf-8')
    baseline=read(ROOT/'docs/quality/current-functional-baseline.json')
    baseline.update(increment=102,evidence_group='finance-fx',verification_script='verify_finance_fx_sources.py',
        backend_evidence_group='finance-fx-backend',backend_top_level_tests=top,backend_result_records=count,
        web_evidence_group='finance-fx',runtime_evidence_group='finance-fx',web_tests=client_counts['web-tests'],mobile_tests=client_counts['mobile-tests'],runtime_tests=client_counts['runtime-tests'],
        development_verification_script='verify_finance_fx_development.py',interaction_kind='explicit_finance_fx_valuation',interaction_sources='finance_fx_sources.py',
        retained_evidence_record='docs/quality/evidence/finance-fx/reused-evidence.json',new_browser_capture=True,new_backend_test_execution=True,full_goal_complete=False,production_ready=False,
        planning_scope_verification_record='docs/quality/evidence/finance-fx/scope-check.json',planning_publication_record='docs/quality/evidence/finance-fx/repository-check.json')
    modified=subprocess.check_output(['git','diff','--name-only','-z'],cwd=ROOT).decode().split('\0')
    added=subprocess.check_output(['git','ls-files','--others','--exclude-standard','-z'],cwd=ROOT).decode().split('\0')
    # This list scopes whitespace checks. Every candidate still receives the
    # repository checker's secret, path, size, link and artifact inspections.
    # Binary media and exact native logs must retain their original bytes.
    published=json.loads(subprocess.check_output(['git','show','624c95d86f737e679e11af4fbb505dedf8e27b71:docs/quality/current-functional-baseline.json'],cwd=ROOT))
    text_suffixes={'.md','.py','.go','.sql','.ts','.tsx','.mjs','.cjs','.js','.json','.toml','.yaml','.yml','.xml','.css','.scss','.html','.gradle','.properties','.txt','.mod','.sum','.kt','.kts','.java','.sh','.ps1'}
    candidates=[*baseline['publication_files'],*modified,*added]
    baseline['publication_files']=sorted(set([*published['publication_files'],*[v for v in candidates if v and (ROOT/v).is_file() and (Path(v).suffix.lower() in text_suffixes or Path(v).name in ('.gitignore','.gitattributes'))]]))
    write(ROOT/'docs/quality/current-functional-baseline.json',baseline)
    print(json.dumps({'passed':True,'increment':102,'backend':[top,count],'clients':client_counts,'media':canonical,'full_goal_complete':False}))

if __name__=='__main__':main()
