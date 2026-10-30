"""Publish the bounded actual-refund feature and current evidence without rewriting history."""
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/refund-fx'
read = lambda p: json.loads(p.read_text(encoding='utf-8'))


def save(path, value):
    previous = path.read_bytes() if path.exists() else b''
    nl = '\r\n' if b'\r\n' in previous else '\n'
    path.write_bytes((json.dumps(value, ensure_ascii=False, indent=2)+'\n').replace('\n',nl).encode())


def prepend(name, body):
    p = ROOT / name
    raw = p.read_bytes()
    assert b'Increment121' not in raw[:1200] and '增量121'.encode() not in raw[:1200]
    first, rest = raw.split(b'\n',1)
    nl = b'\r\n' if first.endswith(b'\r') else b'\n'
    p.write_bytes(first+b'\n'+nl+body.strip().encode().replace(b'\n',nl)+nl+nl+rest.lstrip(b'\r\n'))


def main():
    backend = read(E.parent / 'refund-fx-backend/result.json')
    clients = read(E / 'client-checks.json')
    jvm = read(E / 'android-test-scope.json')
    assert backend['passed'] and clients['passed'] and jvm['passed']
    counts = {v['name']:v['test_count'] for v in clients['checks']}
    overview = '''增量121已接通异币退款的草稿、人工登记、原付款本金扣减、实际币种现金汇总和全额冲销。原付780 HKD、实收100 USD时，现金记录100 USD，原款扣减780 HKD；金额、人工汇率、凭证版本与操作历史分别固定。四端入口使用相同协议。

Increment121 adds actual foreign-currency refund drafts, explicit human confirmation, original-principal limits, received-currency cash reporting and full reversal across all four clients. For 780 HKD originally paid and 100 USD actually refunded, cash records 100 USD while the source principal decreases by 780 HKD. Exact amounts, reviewed rate, evidence versions and history remain pinned.

使用：打开实际付款 → 起草收到退款 → 选择实际实收币种，填写原本金／实际汇率／凭证 → 保存草稿 → 重新核对原付款当前余额并人工确认登记。当前仅使用未分配、未预留原本金；费用／合同异币归属继续待完成，界面与服务端均拒绝越过该边界。冲销保留原实收凭证并恢复原本金；不自动处理汇兑损益或手续费。

Usage: open the original outgoing payment, draft a received refund, enter actual currency and original principal/rate/evidence, then save. Re-read current source availability and explicitly confirm with MFA. Only unallocated, unreserved principal is supported. Foreign expense/contract attribution remains open and is rejected by this protocol. Reversal preserves original evidence and restores principal; no automatic exchange gain/loss or fee posting occurs.

本轮包含完整后端／客户端回归、92→93正常结构升级、实际Web／Taro H5操作和截图／MP4／GIF。Android为开发包检查，JVM缓存状态单独披露；没有真机、模拟器或实网性能测试。327份旧媒体及原证据保留。仍23项待完成（14 P0／9 P1）、5项移出，八项Skill仍draft。仅AI费用控制移出，秘书、TG、授权、隔离、审计与技术资源限制保留。

Checks bind current backend/client sources, a populated 92→93 normal schema upgrade and actual Web/Taro H5 journeys with screenshots, MP4 recordings and GIF tours. Android is a development artifact inspection with JVM cache status disclosed; no physical device, emulator or network benchmark is used. All 327 older media and prior proof bytes are retained. All 23 open packages (14 P0/9 P1), five exclusions and eight draft Skills remain. Only AI fee controls are excluded; secretary/TG authorization, isolation, audit and technical resource limits remain required.'''
    overview += f"\n\n本轮检查 / Checks: Go {backend['discovered_top_level_tests']}个顶层用例、{len(backend['checks'])}条通过结果；Web {counts['web-tests']}；共享移动端 {counts['mobile-tests']}；runtime {counts['runtime-tests']}；JVM {jvm['tests']}（{'缓存复用 / cached' if jvm['gradle_cached'] else '本轮执行 / executed now'}）。正式Linux／云／模型／平台／生产验收仍独立开放。"
    for name in ('README.md','docs/README.md','docs/README.zh-CN.md','docs/README.en.md'):
        prefix = 'docs/' if name == 'README.md' else ''
        links = f'\n\n[设计与使用 / Design and usage]({prefix}development/121-actual-foreign-refunds.md) · [验证 / Verification]({prefix}quality/121-refund-verification.md) · [全部未完成项 / Remaining]({prefix}delivery/03-goal-gap-assessment.md)'
        media = f'\n\n![异币退款 / Foreign refund]({prefix}media/screenshots/refund-fx-web-recorded.png)\n\n![实际页面 / Actual UI]({prefix}media/gifs/refund-fx-web.gif)\n\n[Web MP4]({prefix}media/videos/refund-fx-web.mp4) · [移动端 / Mobile MP4]({prefix}media/videos/refund-fx-mobile.mp4)'
        prepend(name,overview+links+media)
    short = '增量121：GAP-03／18增加未分配本金的异币退款实收登记、原付款本金限额、实际现金汇总及全额冲销；费用／合同异币归属、公司汇差制度与完整业务验收继续开放。仍23 active（14 P0／9 P1）、5 excluded。见[设计](../development/121-actual-foreign-refunds.md)、[验证](../quality/121-refund-verification.md)；当前范围按[修订99](04-current-scope-and-acceptance.md)。'
    for name in ('docs/delivery/04-current-scope-and-acceptance.md','docs/delivery/03-goal-gap-assessment.md','docs/delivery/02-implementation-ledger.md'):
        prepend(name,short)
    for name in ('docs/requirements/05-finance-and-subscriptions.md','docs/development/116-cross-currency-settlement.md','docs/development/118-payment-fee-evidence.md'):
        prepend(name,'增量121当前补充：异币退款已接通未分配、未预留原付款本金的实收登记和全额冲销；原分配的费用／合同异币回转仍待完成，不混用三种币种。普通同币退款沿用原协议，真实费用／现金／手续费依据不改写。详见[异币实收设计](../development/121-actual-foreign-refunds.md)；范围按[修订99](../delivery/04-current-scope-and-acceptance.md)。')
    path = ROOT / 'docs/delivery/04-current-scope.json'
    scope = read(path)
    previous = {r['id']:r['remaining'] for r in scope['items']}
    for row in scope['items']:
        if row['id'] in ('GAP-03','GAP-18'):
            row.setdefault('completed_adaptations',[]).append('ACTUAL-FOREIGN-REFUNDS-121')
            row['completed_detail'] = row.get('completed_detail','')+' 增量121已完成未分配／未预留原本金的异币退款实收、人工汇率凭证、并发本金上限与全额冲销；现金币与原本金独立，费用／合同异币归属仍开放。'
            if row['id'] == 'GAP-03':
                row['remaining'] = '继续确认公司财务制度、跨币种退款的费用／合同归属、汇兑差额会计处理与完整四端财务业务旅程；增量121仅完成未分配原本金的异币实收和冲销，不自动记汇差或手续费。'
    save(path,scope)
    p = ROOT / 'docs/delivery/03-goal-gap-assessment.md'
    lines = p.read_bytes().decode().splitlines(keepends=True)
    for i,line in enumerate(lines):
        if line.startswith('| GAP-03 |'):
            current = next(v for v in scope['items'] if v['id']=='GAP-03')['remaining']
            assert previous['GAP-03'] in line
            lines[i] = line.replace(previous['GAP-03'],current)
    p.write_bytes(''.join(lines).encode())
    prepend('docs/CHANGELOG.md','## Increment121 · 异币退款实收\n\n- 新增人工固定的原付款本金／实收汇率／凭证，四端起草、登记、回执、历史及全额冲销。\n- 迁移0093保留原账本，按原本金核对退款上限，现金报表保留实际币种；费用／合同异币归属明确拒绝。\n- 修复旧移动测试编译器消费共享金额模块的路径，保留全部原用例，修复移动端金额校验提示并新增回归；更新正常升级、源码绑定、实际演示和开发包检查，未扩大用户排除范围。')
    prepend('docs/media/README.md','''增量121实际合成页面：异币实收草稿、确认后的原本金依据、保留凭证的全额冲销。录制在登录后开始，排除密码／MFA和真实业务记录；GIF为实际截图导览，MP4为实际视口录像。

| 页面 | Web | Taro H5 |
| --- | --- | --- |
| 草稿 | [截图](screenshots/refund-fx-web-draft.png) | [截图](screenshots/refund-fx-mobile-draft.png) |
| 已登记 | [截图](screenshots/refund-fx-web-recorded.png) | [截图](screenshots/refund-fx-mobile-recorded.png) |
| 已冲销 | [截图](screenshots/refund-fx-web-reversed.png) | [截图](screenshots/refund-fx-mobile-reversed.png) |
| 演示 | [MP4](videos/refund-fx-web.mp4) · [GIF](gifs/refund-fx-web.gif) | [MP4](videos/refund-fx-mobile.mp4) · [GIF](gifs/refund-fx-mobile.gif) |

327份旧媒体字节保留；当前演示与预检／中断素材分别记录。[验证](../quality/121-refund-verification.md)。''')
    p = ROOT / 'AGENTS.md'
    p.write_bytes(p.read_bytes()+b'\r\n- Increment121 adds actual foreign-currency refund drafts, reviewed unallocated original principal, cash reports and immutable full reversal across four clients, schema93. Reject foreign expense/contract attribution; keep company FX policy and full GAP-03/18 acceptance open. Preserve source-bound evidence, legacy ledger bytes and the AI fee-only exclusion. See `docs/development/121-actual-foreign-refunds.md`.\r\n')
    p = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(p)
    assert baseline['increment'] == 120
    baseline.update(increment=121,evidence_group='refund-fx',mobile_tests=counts['mobile-tests'],
        verification_script='verify_refund_fx_sources.py',backend_evidence_group='refund-fx-backend',
        backend_top_level_tests=backend['discovered_top_level_tests'],backend_result_records=len(backend['checks']),
        web_evidence_group='refund-fx',runtime_evidence_group='refund-fx',web_tests=counts['web-tests'],runtime_tests=counts['runtime-tests'],
        development_verification_script='verify_refund_fx_development.py',interaction_kind='actual_foreign_refunds',interaction_sources='refund_fx_sources.py',
        retained_evidence_record='docs/quality/evidence/refund-fx/reused-evidence.json',dependency_audit_record='docs/quality/evidence/refund-fx/dependency-audit.json',
        planning_scope_verification_record='docs/quality/evidence/refund-fx/scope-check.json',planning_publication_record='docs/quality/evidence/refund-fx/repository-check.json')
    changed = subprocess.check_output(['git','diff','--name-only','-z'],cwd=ROOT).decode().split('\0')
    new = subprocess.check_output(['git','ls-files','--others','--exclude-standard','-z'],cwd=ROOT).decode().split('\0')
    names = set(baseline['publication_files']+[n for n in changed+new if n and (ROOT/n).is_file()])
    names.update('docs/quality/evidence/refund-fx/'+n for n in ('final-source-check.json','scope-check.json','acceptance-applicability-check.json'))
    baseline['publication_files'] = sorted(n for n in names if Path(n).suffix.lower() not in {'.png','.gif','.mp4','.webm','.log'} and n!=baseline['planning_publication_record'])
    save(p,baseline)
    print(json.dumps({'passed':True,'increment':121,'active':23,'excluded':5}))


if __name__ == '__main__': main()
