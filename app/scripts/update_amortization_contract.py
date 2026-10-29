"""Add the optional cost allocation schedule without rewriting old CSV shapes."""
import copy
import json
from pathlib import Path

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent


def main():
    path = ROOT / 'docs/development/openapi.json'
    document = json.loads(path.read_text(encoding='utf-8'))
    schemas = document['components']['schemas']
    ref = lambda name: {'$ref': '#/components/schemas/' + name}
    def obj(properties, required=None):
        return {'type': 'object', 'properties': properties, 'additionalProperties': False,
                'required': list(properties) if required is None else required}
    uuid = {'type': 'string', 'format': 'uuid'}
    date = {'type': 'string', 'format': 'date'}
    number = {'type': 'integer', 'minimum': 1, 'maximum': 9007199254740991}
    digest = {'type': 'string', 'pattern': '^[a-f0-9]{64}$'}
    amount = copy.deepcopy(schemas['CostContent']['properties']['amount']['anyOf'][0])
    method = {'type': 'string', 'enum': ['monthly_equal', 'daily_weighted']}
    allocations = copy.deepcopy(schemas['CostContent']['properties']['allocations'])
    period = obj({'sequence': {'type': 'integer', 'minimum': 1, 'maximum': 120},
                  'service_from': date, 'service_to': date, 'recognized_on': date,
                  'amount': amount, 'shares': allocations})
    schemas['AmortizationPeriod'] = period
    schemas['AmortizationContent'] = obj({
        'cost_id': uuid, 'cost_version': number, 'cost_revision_id': uuid, 'cost_sha256': digest,
        'currency': {'type': 'string', 'pattern': '^[A-Z]{3}$'},
        'exponent': {'type': 'integer', 'minimum': 0, 'maximum': 6}, 'amount': amount,
        'method': method, 'rounding': {'const': 'allocation_cumulative_half_up'},
        'service_from': date, 'service_to': date,
        'periods': {'type': 'array', 'minItems': 1, 'maxItems': 120, 'items': ref('AmortizationPeriod')}})
    base = copy.deepcopy(schemas['CostConfirm']['properties'])
    base['expected_plan_version'] = {'type': 'integer', 'minimum': 0, 'maximum': 9007199254740990}
    fields = {**base, 'method': method, 'service_from': date, 'service_to': date}
    schemas['AmortizationInput'] = obj(fields)
    schemas['AmortizationPublish'] = obj({**fields, 'preview_sha256': digest, 'acknowledged': {'const': True}})
    schemas['AmortizationStop'] = obj({**base, 'acknowledged': {'const': True}})
    schemas['AmortizationPreview'] = obj({'plan_version': number, 'content_sha256': digest, 'content': ref('AmortizationContent')})
    schemas['AmortizationVersion'] = obj({
        'id': uuid, 'cost_id': uuid, 'version': number, 'state': {'type': 'string', 'enum': ['active','stopped']},
        'content_sha256': digest, 'period_count': {'type': 'integer', 'minimum': 1, 'maximum': 120},
        'actor_id': uuid, 'reason': base['reason'], 'created_at': {'type': 'string', 'format': 'date-time'},
        'content': ref('AmortizationContent')})
    schemas['AmortizationVersion']['required'].remove('content')
    schemas['AmortizationOverview'] = obj({'cost_id': uuid,
        'version': {'type': 'integer', 'minimum': 0, 'maximum': 9007199254740991},
        'can_manage': {'type': 'boolean'},
        'current': {'anyOf': [ref('AmortizationVersion'), {'type':'null'}]}})
    schemas['AmortizationEvidence'] = obj({'plan_id': uuid, 'plan_version': number,
        'plan_sha256': digest, 'sequence': period['properties']['sequence'], 'method': method,
        'service_from': date, 'service_to': date, 'original_date': date})
    for name in ('AmortizationPreview','AmortizationVersion','AmortizationOverview'):
        schemas[name+'Response'] = obj({'data':ref(name),'meta':ref('Meta')})
    page = copy.deepcopy(schemas['FinanceFXBookListResponse'])
    page['properties']['data']['properties']['items']['items'] = ref('AmortizationVersion')
    schemas['AmortizationVersionListResponse'] = page
    summary = copy.deepcopy(schemas['CostReport'])
    summary['properties']['kind'] = {'const':'amortization'}
    summary['properties']['basis'] = {'const':'latest_active_schedules'}
    summary['required'].append('basis')
    schemas['AmortizationReport'] = summary
    kinds = schemas['FinancialReportQuery']['properties']['kind']['enum']
    if 'amortization' not in kinds:
        kinds.append('amortization')
    kinds = schemas['FinancialReportSource']['properties']['source_type']['enum']
    if 'cost_amortization' not in kinds:
        kinds.append('cost_amortization')
    kinds = schemas['FinancialReportSource']['properties']['entry_kind']['enum']
    if 'amortization' not in kinds:
        kinds.append('amortization')
    schemas['FinancialReportSource']['properties']['amortization'] = ref('AmortizationEvidence')
    if ref('AmortizationReport') not in schemas['FinancialReport']['properties']['summary']['anyOf']:
        schemas['FinancialReport']['properties']['summary']['anyOf'].append(ref('AmortizationReport'))
    def route(operation, response, request=None, paged=False, version=False, status='200'):
        model = copy.deepcopy(document['paths']['/costs/{id}/confirm']['post'] if request else document['paths']['/costs/{id}']['get'])
        model['operationId'] = model['summary'] = operation
        model['tags'] = ['finance']
        model['parameters'] = [p for p in model['parameters'] if not isinstance(p,dict) or p.get('in')!='query']
        if version:
            model['parameters'].append({'name':'version','in':'path','required':True,'schema':uuid})
        if paged:
            model['parameters'] += copy.deepcopy(document['paths']['/finance/fx-books']['get']['parameters'][:2])
        model['responses'] = {k:v for k,v in model['responses'].items() if not k.startswith('2')}
        model['responses'][status] = {'description':'Success','content':{'application/json':{'schema':ref(response)}}}
        if request:
            model['requestBody']['content']['application/json']['schema'] = ref(request)
        return model
    prefix = '/costs/{id}/amortization'
    document['paths'][prefix] = {
        'get': route('getCostAmortization','AmortizationOverviewResponse'),
        'post': route('publishCostAmortization','AmortizationVersionResponse','AmortizationPublish',status='201')}
    document['paths'][prefix+'/preview'] = {'post':route('previewCostAmortization','AmortizationPreviewResponse','AmortizationInput')}
    document['paths'][prefix+'/stop'] = {'post':route('stopCostAmortization','AmortizationVersionResponse','AmortizationStop',status='201')}
    document['paths'][prefix+'/versions'] = {'get':route('listCostAmortizationVersions','AmortizationVersionListResponse',paged=True)}
    document['paths'][prefix+'/versions/{version}'] = {'get':route('getCostAmortizationVersion','AmortizationVersionResponse',version=True)}
    content = json.dumps(document,ensure_ascii=False,indent=2)+'\n'
    path.write_text(content,encoding='utf-8')
    (APP / 'backend/internal/contract/openapi.json').write_bytes(path.read_bytes())


if __name__ == '__main__':
    main()
