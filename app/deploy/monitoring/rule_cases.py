"""Generate promtool inputs: sustained failure, missing data and rule recovery."""
def cases(rule_file, target_ids, units):
    labels=lambda target:{'severity':'critical','target_id':target}
    summary={'summary':'Independent infrastructure check requires operator review.'}
    base=[{'series':f'probe_success{{job="copypaste_blackbox",target_id="{target}"}}','values':'1+0x10'} for target in target_ids]
    base += [{'series':f'node_systemd_unit_state{{job="copypaste_host",name="{unit}",state="active"}}','values':'1+0x10'} for unit in units]
    tests=[]
    def test(name, values, time, alert, expected, key='api_ready', kind='target'):
        rows=[dict(row) for row in base]
        needle=f'target_id="{key}"' if kind=='target' else f'name="{key}"'
        next(row for row in rows if needle in row['series'])['values']=values
        tests.append({'name':name,'interval':'1m','input_series':rows,'alert_rule_test':[{
            'eval_time':time,'alertname':alert,'exp_alerts':expected}]})
    target='api_ready'
    test('one bad sample does not fire','0 1+0x9','1m','CopyPasteEndpointDown',[])
    test('sustained API failure fires','0+0x10','2m','CopyPasteEndpointDown',[{'exp_labels':{'job':'copypaste_blackbox',**labels(target)},'exp_annotations':summary}])
    test('rule resolves after healthy observation','0 0 0 1+0x7','3m','CopyPasteEndpointDown',[])
    test('missing observation is unknown','1 stale _ _ _ _ _ _ _ _','4m','CopyPasteObservationUnknown',[{'exp_labels':{'job':'copypaste_blackbox',**labels(target)},'exp_annotations':summary}])
    unit=units[0]
    test('required application unit failure','0+0x10','2m','CopyPasteRequiredUnitDown',[{'exp_labels':{'job':'copypaste_host','name':unit,'state':'active','severity':'critical','unit':unit},'exp_annotations':summary}],unit,'unit')
    return {'rule_files':[str(rule_file)],'evaluation_interval':'1m','tests':tests}
