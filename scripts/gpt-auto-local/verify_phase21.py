"""Phase 2.1 acceptance: public/admin HTTP through isolated Nginx; SQL assertions only.
Restores local global live routing to OFF even on failure. Never contacts real Jev.
"""
import concurrent.futures
import csv
import io
import json
import pathlib
import sys
import time
import uuid
from decimal import Decimal as D
import verify_phase2 as p
v = p.v
OUT = v.ROOT / 'artifacts/gpt-auto-phase21'
OUT.mkdir(parents=True, exist_ok=True)
p.OUT = v.OUT = OUT
POOL = ['gpt-5.6-luna', 'gpt-6-luna', 'gpt-6.1-sol']
TARIFF = dict(mode='virtual_model_fixed_rates', input_price_per_mtok='2', cached_input_price_per_mtok='0.5', output_price_per_mtok='8', cache_write_price_per_mtok=None)

def group(**changes):
    return v.api(f"admin/groups/{p.state['group_id']}", changes, 'PUT')

def tariff(pricing=TARIFF):
    state = v.api('admin/gpt-auto/pricing')
    return v.api('admin/gpt-auto/pricing', dict(pricing=pricing, expected_version=state['pricing']['version'], confirm=True), 'PUT')

def setup():
    result = p.setup()
    old_id = int(v.sql('SELECT COALESCE(max(id),0) FROM usage_logs'))
    old_sum = v.sql(f'SELECT count(*),sum(actual_cost)::text FROM usage_logs WHERE id<={old_id}')
    p.state.update(historical_id=old_id,historical_sum=old_sum)
    p.live_config(live_enabled=False, internal_only=True, allowed_api_keys=[])
    group(gpt_auto_enabled=False, model_allowlist={'enabled':False, 'models':[]}, rate_multiplier=1, model_rate_multipliers={})
    return result

def defaults_and_migration():
    # Every existing group started off; no automatic enrollment in migration 282.
    sql = v.sql("SELECT column_default FROM information_schema.columns WHERE table_name='groups' AND column_name='gpt_auto_enabled'")
    assert sql == 'false', sql
    g = v.api('admin/groups', dict(name='phase21-default-'+str(time.time_ns()), platform='openai', rate_multiplier=1), 'POST')
    assert g['gpt_auto_enabled'] is False
    return {'new_group_default':False, 'database_default':sql}

def denied_switch():
    p.live_config(live_enabled=True, internal_only=True, allowed_api_keys=[p.state['key_a_id']])
    tariff()
    group(gpt_auto_enabled=False)
    v.control(reset=True)
    assert 'gpt-auto' not in v.http('/v1/models',auth=v.key)['body']
    for endpoint, body in [('chat/completions',p.auto_chat()),('responses',p.auto_responses())]:
        r = p.gateway(endpoint,body)
        assert r['status']==404 and r['data']['error']['code']=='model_not_found',r
    assert not p.mock_requests(), 'denial must not call Jev or upstream'
    return {'endpoints':2,'status':404,'classifier_calls':0}

def enabled_and_rollout():
    group(gpt_auto_enabled=True)
    assert v.api(f"admin/groups/{p.state['group_id']}")['gpt_auto_enabled'] is True
    assert 'gpt-auto' in v.http('/v1/models',auth=v.key)['body']
    assert p.gateway('responses',p.auto_responses(),auth=p.state['key_b'])['status']==404
    p.live_config(internal_only=False,allowed_groups=[99999999])
    assert p.gateway('chat/completions',p.auto_chat())['status']==404, 'listed key cannot bypass group rollout'
    p.live_config(allowed_groups=[])
    assert p.gateway('chat/completions',p.auto_chat())['status']==200
    p.live_config(internal_only=True,allowed_api_keys=[p.state['key_a_id']])
    return {'switch_saved':True,'internal_only':True,'rollout_enforced':True}

def validate_pricing():
    state = v.api('admin/gpt-auto/pricing')
    for bad in [dict(TARIFF,output_price_per_mtok=None),dict(TARIFF,input_price_per_mtok='-1'),dict(TARIFF,input_price_per_mtok='NaN'),dict(TARIFF,input_price_per_mtok='1001'),dict(TARIFF,input_price_per_mtok='0.0000001')]:
        r=v.http('/api/v1/admin/gpt-auto/pricing',dict(pricing=bad,expected_version=state['pricing']['version'],confirm=True),'PUT',v.token)
        assert r['status']==400,r['body']
    r=v.http('/api/v1/admin/gpt-auto/pricing',dict(pricing=TARIFF,expected_version=state['pricing']['version'],confirm=False),'PUT',v.token)
    assert r['status']==400
    r=v.http('/api/v1/admin/gpt-auto/pricing',dict(pricing=TARIFF,expected_version=-1,confirm=True),'PUT',v.token)
    assert r['status']==409
    assert v.http('/api/v1/admin/gpt-auto/pricing',auth=p.state['user_token'])['status']==403
    return {'invalid_tariffs_rejected':5,'confirmation_required':True,'stale_version_status':409,'admin_only':True}

def fixed_billing():
    # Explicit fixture prices for executed models make procurement differences deterministic.
    entries=[dict(models=[model],billing_mode='token',input_price=(i+1)*0.000001,cache_read_price=(i+1)*0.0000001,output_price=(i+1)*0.000003) for i,model in enumerate(POOL)]
    group(model_pricing=entries)
    v.control(input_tokens=1000,output_tokens=500,cached=200,upstream='ok',jev='ok')
    rows=[]
    for model in POOL:
        v.control(jev_model=model,jev_effort='medium')
        r=p.gateway('responses',p.auto_responses('fixed tariff '+model))
        assert r['status']==200,r['body']
        row=p.wait_usage(p.rid(r));p.decision_row(p.rid(r))
        assert row[0]==model and row[1]=='gpt-auto',row
        assert D(row[2])==D('0.0057'),row
        assert list(map(int,row[6:9]))==[800,200,500],row
        assert D(row[4])==1,row
        meta=v.sql("SELECT pricing_model,pricing_mode,pricing_config_version,tariff_input_per_mtok FROM gpt_auto_shadow_decisions WHERE request_id='client:"+p.rid(r)+"'")
        assert meta.startswith('gpt-auto|virtual_model_fixed_rates|'),meta
        rows.append({'model':model,'charge':row[2],'list_cost':row[3],'pricing_metadata':meta,'request_id':p.rid(r)})
    assert len({r['list_cost'] for r in rows})==3,rows
    before=D(v.sql(f"SELECT balance FROM users WHERE id={p.state['user_a_id']}"))
    r=p.gateway('responses',p.auto_responses('deduct once'));row=p.wait_usage(p.rid(r))
    after=v.wait(lambda: (value:=D(v.sql(f"SELECT balance FROM users WHERE id={p.state['user_a_id']}")))!=before and value,15)
    assert before-after==D('0.0057'),(before,after,row)
    return {'rows':rows,'balance_delta':str(before-after)}

def group_pricing():
    group(rate_multiplier=0.8)
    r=p.gateway('responses',p.auto_responses('group discount'))
    row=p.wait_usage(p.rid(r)); assert D(row[2])==D('0.00456'),row
    group(model_pricing=[dict(models=['gpt-auto'],billing_mode='token',input_price=0.000001,cache_read_price=0.00000025,output_price=0.000004)])
    r=p.gateway('responses',p.auto_responses('group override'))
    assert D(p.wait_usage(p.rid(r))[2])==D('0.00228')
    state=v.api('admin/gpt-auto/pricing');view=next(g for g in state['groups'] if g['group_id']==p.state['group_id'])
    assert view['pricing_source']=='group_override' and D(view['effective_input_price_per_mtok'])==D('0.8'),view
    group(rate_multiplier=1,model_pricing=[])
    return {'discount_charge':'0.00456','override_charge':'0.00228','preview':view}

def snapshot_stream_partial():
    tariff()
    old=v.api('admin/gpt-auto/pricing')['pricing']['version']
    v.control(stream_delay=1,upstream='ok')
    with concurrent.futures.ThreadPoolExecutor() as pool:
        future=pool.submit(p.gateway,'responses',p.auto_responses('stream while pricing changes',stream=True),stream=True)
        v.wait(lambda:any(r.get('stream') for r in p.mock_requests()[-2:] if not r['path'].endswith('/systemone')),5)
        time.sleep(0.2)
        tariff(dict(TARIFF,output_price_per_mtok='80'))
        r=future.result()
    v.control(stream_delay=0)
    assert r['status']==200 and 'response.completed' in r['body']
    row=p.wait_usage(p.rid(r));assert D(row[2])==D('0.0057'),row
    version=v.sql("SELECT pricing_config_version FROM gpt_auto_shadow_decisions WHERE request_id='client:"+p.rid(r)+"'")
    assert int(version)==old,(version,old)
    tariff()
    v.control(upstream='partial')
    r=p.gateway('responses',p.auto_responses('partial usage',stream=True),stream=True)
    row=p.wait_usage(p.rid(r));assert D(row[2])==D('0.0057'),row
    v.control(upstream='ok')
    return {'snapshot_version':old,'stream_charge':'0.0057','partial_usage_charge':row[2]}

def underlying_permissions():
    group(model_allowlist={'enabled':True,'models':['gpt-auto','gpt-6-luna']})
    v.control(jev_model='gpt-6.1-sol')
    r=p.gateway('responses',p.auto_responses('authorized candidate only'))
    assert r['status']==200,r['body']
    assert p.last_upstream()['model']=='gpt-6-luna'
    group(model_allowlist={'enabled':True,'models':['gpt-6-luna']})
    before=len(p.mock_requests())
    assert p.gateway('responses',p.auto_responses())['status'] in (403,404)
    assert len(p.mock_requests())==before
    group(model_allowlist={'enabled':False,'models':[]})
    return {'forbidden_recommendation_rerouted_to':'gpt-6-luna','denied_without_virtual_entitlement':True}

def revoke_affinity():
    v.control(jev='ok',upstream='ok',jev_model='gpt-6-luna')
    r=p.gateway('responses',p.auto_responses('remember',prompt_cache_key='phase21-revoke'))
    assert r['status']==200
    group(gpt_auto_enabled=False)
    v.control(reset=True)
    assert 'gpt-auto' not in v.http('/v1/models',auth=v.key)['body']
    r=p.gateway('responses',p.auto_responses('reuse revoked',prompt_cache_key='phase21-revoke'))
    assert r['status']==404,r['body']
    assert not p.mock_requests()
    direct=p.gateway('responses',dict(model='gpt-6.1-sol',input='ordinary model'))
    assert direct['status']==200,direct['body']
    group(gpt_auto_enabled=True)
    return {'revoked_immediately':True,'jev_calls':0,'ordinary_model_status':200}

def idempotency_and_unsupported():
    group(rate_multiplier=1,model_pricing=[])
    v.control(input_tokens=1000,output_tokens=500,cached=200,jev='ok',upstream='ok')
    request_id=str(uuid.uuid4())
    body=p.auto_responses('identical idempotent request')
    before=D(v.sql(f"SELECT balance FROM users WHERE id={p.state['user_a_id']}"))
    ids=[]
    for _ in range(2):
        r=p.gateway('responses',body,headers={'X-Client-Request-ID':request_id})
        assert r['status']==200,r['body']
        ids.append(p.rid(r));p.wait_usage(ids[-1])
    assert len(set(ids))==2, 'gateway deliberately generates its own request IDs'
    time.sleep(0.3)
    counts=[v.sql(f"SELECT count(*) FROM usage_logs WHERE request_id='client:{rid}'") for rid in ids]
    after=D(v.sql(f"SELECT balance FROM users WHERE id={p.state['user_a_id']}"))
    assert counts==['1','1'] and before-after==D('0.0114'),(counts,before,after)
    count=','.join(counts)
    v.control(reset=True)
    r=p.gateway('responses',p.auto_responses(tools=[{'type':'web_search'}]))
    assert r['status']==400 and not p.mock_requests(),r
    return {'settlement_rows':count,'balance_delta':str(before-after),'unpriced_tool_status':400,'jev_calls_on_rejection':0}

def legacy_compatibility():
    v.control(input_tokens=None,output_tokens=None,cached=0)
    tariff(dict(mode='legacy_executed_model_multiplier'))
    result=p.billing()
    tariff()
    return result

def exports():
    result=p.exports()
    with (OUT/'admin-export.csv').open(encoding='utf-8-sig') as f:
        rows=list(csv.DictReader(f))
    fixed=[r for r in rows if r.get('pricing_mode')=='virtual_model_fixed_rates' and r['billing_status']=='final']
    assert fixed
    for r in fixed:
        assert r['pricing_model']=='gpt-auto'
        assert r['export_schema']=='gpt-auto-export-v2'
        assert abs(D(r['gross_profit_usd'])-(D(r['customer_charge_usd'])-D(r['upstream_cost_usd'])))<=D('0.0000000001')
    result['fixed_records']=len(fixed)
    return result

def historical_unchanged():
    after=v.sql(f"SELECT count(*),sum(actual_cost)::text FROM usage_logs WHERE id<={p.state['historical_id']}")
    assert after==p.state['historical_sum'],(after,p.state['historical_sum'])
    return {'count_and_total':after,'max_historical_id':p.state['historical_id']}

def main():
    try:
        if not p.check('setup',setup):return 1
        checks=[('migration_and_new_group_off',defaults_and_migration),('disabled_group_no_listing_no_jev',denied_switch),('enabled_group_internal_and_rollout',enabled_and_rollout),('pricing_validation_admin_confirmation',validate_pricing),('same_charge_three_models_procurement_and_balance',fixed_billing),('group_discount_and_override',group_pricing),('midstream_snapshot_and_partial_settlement',snapshot_stream_partial),('underlying_model_permissions',underlying_permissions),('revocation_beats_affinity',revoke_affinity),('generated_request_ids_single_settlement_and_unpriced_tools',idempotency_and_unsupported),('legacy_compatibility',legacy_compatibility),('chat_responses_sse_tools',p.endpoints_streaming_tools),('session_affinity',p.affinity),('timeout_fallback_and_upstream_failure',p.failures),('client_cancellation',p.client_cancellation),('concurrency',p.concurrency),('exports_and_customer_isolation',exports)]
        for name,fn in checks:p.check(name,fn)
    finally:
        if v.token:
            p.live_config(live_enabled=False,internal_only=True)
            if p.state:
                group(gpt_auto_enabled=False,rate_multiplier=1,model_pricing=[])
                if 'historical_id' in p.state:
                    p.check('historical_charges_unchanged',lambda: historical_unchanged())
        v.control(jev='ok',upstream='ok',stream_delay=0,input_tokens=None,output_tokens=None,cached=0)
        summary={'scenarios':len(p.results),'passed':sum(r['passed'] for r in p.results),'failed':[r['name'] for r in p.results if not r['passed']]}
        (OUT/'phase21-results.json').write_text(json.dumps(p.results,indent=2,ensure_ascii=False))
        (OUT/'phase21-summary.json').write_text(json.dumps(summary,indent=2))
        print(json.dumps(summary),flush=True)
    return 0 if not summary['failed'] else 1
if __name__=='__main__':sys.exit(main())
