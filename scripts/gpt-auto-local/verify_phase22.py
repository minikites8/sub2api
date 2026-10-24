"""Phase 2.2: dynamic routing acceptance through isolated Nginx.
No external providers. Restores local live/dynamic/group switches OFF.
"""
import concurrent.futures
import csv
import io
import json
import statistics
import time
import uuid
from decimal import Decimal as D
import verify_phase21 as q
p, v = q.p, q.v
OUT = v.ROOT / 'artifacts/gpt-auto-phase22'
OUT.mkdir(parents=True, exist_ok=True)
p.OUT = v.OUT = OUT
start_time = time.time()

def dynamic(enabled=True, **changes):
    cfg = v.api('admin/gpt-auto/config')['live']['dynamic_routing']
    cfg.update(enabled=enabled, **changes)
    return p.live_config(dynamic_routing=cfg)

def setup():
    result = q.setup()
    assert v.api('admin/gpt-auto/config')['live']['dynamic_routing']['enabled'] is False
    q.tariff();q.group(gpt_auto_enabled=True)
    p.live_config(live_enabled=True,internal_only=True,allowed_api_keys=[p.state['key_a_id']],live_request_budget=0,classifier_timeout_ms=1500)
    dynamic()
    v.control(reset=True,jev='ok',upstream='ok',input_tokens=1000,cached=200,output_tokens=500,unique_ids=True)
    # Different procurement rates; independent tariff remains $0.0057.
    q.group(model_pricing=[dict(models=[m],billing_mode='token',input_price=(i+1)*0.000001,cache_read_price=(i+1)*0.0000001,output_price=(i+1)*0.000003) for i,m in enumerate(q.POOL)])
    return result

def detail(r):
    rid=p.rid(r);p.decision_row(rid)
    raw=v.sql("SELECT row_to_json(t) FROM (SELECT final_model,final_effort,jev_model,jev_effort,features,request_id,classifier_cost,classifier_latency_ms FROM gpt_auto_shadow_decisions WHERE request_id='client:"+rid+"') t")
    return json.loads(raw)

def calls(): return sum(x['path'].endswith('/systemone') for x in p.mock_requests())

def workflow():
    session='phase22-'+uuid.uuid4().hex
    turns=[('gpt-6-luna','low'),('gpt-6.1-sol','high'),('gpt-6.1-sol','medium'),('gpt-6-luna','low'),('gpt-5.6-luna','none'),('gpt-5.6-luna','none')]
    expected=['gpt-6-luna','gpt-6.1-sol','gpt-6.1-sol','gpt-6.1-sol','gpt-6.1-sol','gpt-5.6-luna']
    messages=[{'role':'user','content':'Implement a worker, investigate failures, repair it, then update documentation.'}]
    stages=['routine implementation','tool execution failed','diagnose a cross-component deadlock','repair verified; prepare documentation','write concise documentation','finish concise documentation']
    before=calls();out=[]
    for i,((model,effort),stage,want) in enumerate(zip(turns,stages,expected)):
        v.control(jev_model=model,jev_effort=effort)
        messages.append({'role':'assistant','content':stage})
        body={'model':'gpt-auto','messages':messages,'prompt_cache_key':session,'stream':i%2==1}
        r=p.gateway('chat/completions',body,headers={'session-id':session},stream=i%2==1)
        assert r['status']==200,r['body']
        if body['stream']:assert 'data:' in r['body'] and '[DONE]' in r['body']
        assert p.last_upstream()['model']==want,p.last_upstream()
        row=detail(r);t=row['features']['switching']
        assert row['jev_model']==model and row['jev_effort']==effort,row
        assert row['final_model']==want and t['affinity_type']=='soft',row
        assert t['session_decision_sequence']==i+1,t
        usage=p.wait_usage(p.rid(r));assert D(usage[2])==D('0.0057'),usage
        out.append(dict(jev=model,executed=want,effort=row['final_effort'],switch=t['switch_direction'],blocked=t.get('switch_block_reason'),charge=usage[2],procurement_basis=usage[3],request_id=p.rid(r)))
    assert calls()-before==len(turns)
    assert len({x['procurement_basis'] for x in out})==3
    return {'turns':out,'jev_calls':calls()-before}

def hard_state():
    v.control(jev_model='gpt-6-luna',jev_effort='low')
    first=p.gateway('responses',p.auto_responses('stateful initial'))
    assert first['status']==200,first
    ref=first['data']['id'];before=calls()
    v.control(jev_model='gpt-6.1-sol',jev_effort='high')
    r=p.gateway('responses',p.auto_responses('continue',previous_response_id=ref))
    assert r['status']==200,r['body']
    t=detail(r)['features']['switching']
    assert t['affinity_type']=='hard' and t['state_dependency']=='hard_model_and_account',t
    assert calls()==before and p.last_upstream()['model']=='gpt-6-luna'
    assert p.last_upstream()['previous_response_id']==ref
    for extra in [dict(previous_response_id='missing-state'),dict(conversation='unverified-conversation')]:
        denied=p.gateway('responses',p.auto_responses('continue',**extra))
        assert denied['status']==409 and denied['data']['error']['code']=='gpt_auto_state_continuation_unavailable',denied
    assert calls()==before
    return {'binding_preserved':True,'missing_binding_status':409,'jev_calls_on_continuation':0}

def permissions():
    session='revoke-'+uuid.uuid4().hex
    v.control(jev_model='gpt-6.1-sol',jev_effort='high')
    r=p.gateway('chat/completions',p.auto_chat(),headers={'session-id':session});assert r['status']==200
    q.group(model_allowlist={'enabled':True,'models':['gpt-auto','gpt-6-luna']})
    r=p.gateway('chat/completions',p.auto_chat(),headers={'session-id':session})
    assert r['status']==200 and p.last_upstream()['model']=='gpt-6-luna',r
    q.group(gpt_auto_enabled=False);before=calls()
    assert 'gpt-auto' not in v.http('/v1/models',auth=v.key)['body']
    r=p.gateway('chat/completions',p.auto_chat(),headers={'session-id':session})
    assert r['status']==404 and calls()==before
    q.group(gpt_auto_enabled=True,model_allowlist={'enabled':False,'models':[]})
    return {'model_revocation_respected':True,'group_off':404,'denial_jev_calls':0}

def concurrent_session():
    dynamic(downgrade_stability_count=2)
    session='concurrent-'+uuid.uuid4().hex
    v.control(jev_model='gpt-6.1-sol',jev_effort='high')
    assert p.gateway('chat/completions',p.auto_chat(),headers={'session-id':session})['status']==200
    v.control(jev_model='gpt-6-luna',jev_effort='low',upstream='delay',delay=0.2)
    before=calls()
    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
        responses=list(pool.map(lambda _:p.gateway('chat/completions',p.auto_chat(),headers={'session-id':session}),range(8)))
    v.control(upstream='ok')
    assert all(r['status']==200 for r in responses),[r['status'] for r in responses]
    assert calls()-before==8
    assert len({p.rid(r) for r in responses})==8
    for r in responses:assert D(p.wait_usage(p.rid(r))[2])==D('0.0057')
    return {'requests':8,'classifications':8,'unique_settlements':8}

def exports():
    end=time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime(time.time()+60))
    start=time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime(start_time))
    query=f'start_time={start}&end_time={end}&mode=live'
    result={}
    for fmt in ['csv','jsonl']:
        status,_,body=p.raw_get('/api/v1/admin/gpt-auto/decisions/export?format='+fmt+'&'+query,v.token)
        assert status==200
        (OUT/('admin-export.'+fmt)).write_bytes(body)
        rows=list(csv.DictReader(io.StringIO(body.decode('utf-8-sig')))) if fmt=='csv' else [json.loads(l) for l in body.splitlines()]
        rows=[r for r in rows if r.get('affinity_type')]
        assert rows and all(r['export_schema']=='gpt-auto-export-v3' for r in rows)
        assert any(r['switch_direction']=='upgrade' for r in rows)
        assert any(r['switch_direction']=='downgrade' for r in rows)
        assert any(r['affinity_type']=='hard' for r in rows)
        result[fmt]=len(rows)
    status,_,body=p.raw_get('/api/v1/gpt-auto/routing-logs/export?format=jsonl&'+query,p.state['user_token'])
    assert status==200
    for line in body.splitlines():
        row=json.loads(line)
        assert not any(k in row for k in ['upstream_account_id','upstream_cost_usd','session_correlation_id','switch_block_reason'])
    return result

def redis_counts():
    raw=v.dc('exec','-T','redis','redis-cli','INFO','commandstats')
    return {line.split(':',1)[0]:int(line.split('calls=')[1].split(',')[0]) for line in raw.splitlines() if 'calls=' in line}

def performance():
    modes={}
    for enabled in [False,True]:
        dynamic(enabled,downgrade_stability_count=2)
        session='benchmark-'+uuid.uuid4().hex
        before_id=int(v.sql('SELECT COALESCE(max(id),0) FROM gpt_auto_shadow_decisions'))
        before=calls();latencies=[];redis_before=redis_counts()
        for i in range(100):
            model=['gpt-6-luna','gpt-6.1-sol','gpt-6-luna','gpt-6-luna','gpt-5.6-luna','gpt-5.6-luna'][i%6]
            v.control(jev='ok',jev_model=model,jev_effort='low')
            r=p.gateway('chat/completions',p.auto_chat('same synthetic agent task'),headers={'session-id':session})
            assert r['status']==200,r['body'][:400]
            latencies.append(r['ms'])
        p.decision_row(p.rid(r));p.wait_usage(p.rid(r))
        stats=json.loads(v.sql(f"SELECT row_to_json(t) FROM (SELECT count(*) AS turns,sum(classifier_cost) AS classifier_cost,avg(classifier_latency_ms) AS avg_classification_ms,percentile_cont(0.5) within group(order by classifier_latency_ms) AS p50_ms,percentile_cont(0.95) within group(order by classifier_latency_ms) AS p95_ms,percentile_cont(0.99) within group(order by classifier_latency_ms) AS p99_ms FROM gpt_auto_shadow_decisions WHERE id>{before_id}) t"))
        stats['redis_command_deltas']={k:n-redis_before.get(k,0) for k,n in redis_counts().items() if n!=redis_before.get(k,0)}
        stats.update(jev_calls=calls()-before,average_request_ms=statistics.mean(latencies),p95_request_ms=sorted(latencies)[94])
        stats['distribution']=v.sql(f"SELECT final_model,count(*) FROM gpt_auto_shadow_decisions WHERE id>{before_id} GROUP BY final_model")
        stats['switches']=v.sql(f"SELECT features->'switching'->>'switch_direction',count(*) FROM gpt_auto_shadow_decisions WHERE id>{before_id} GROUP BY 1")
        stats['settled']=v.sql(f"SELECT count(*),sum(u.actual_cost),sum(COALESCE(u.account_stats_cost,u.total_cost*COALESCE(u.account_rate_multiplier,1))) FROM usage_logs u JOIN gpt_auto_shadow_decisions d ON d.request_id=u.request_id AND d.api_key_id=u.api_key_id WHERE d.id>{before_id}")
        assert stats['jev_calls']==(100 if enabled else 1),stats
        modes['dynamic' if enabled else 'legacy']=stats
    return modes


def no_upstream_and_effort():
    dynamic()
    v.control(jev='ok',upstream='ok',jev_model='gpt-6.1-sol',jev_effort='none')
    r=p.gateway('responses',p.auto_responses('unsupported Sol effort normalized'))
    assert r['status']==200 and p.upstream_effort(p.last_upstream())=='low',r
    try:
        assert v.api(f'admin/accounts/{v.account_id}/schedulable',{'schedulable':False},'POST')['schedulable'] is False
        r=p.gateway('chat/completions',p.auto_chat('no schedulable upstream'))
        assert r['status'] in (400,404,503),r['body'][:400]
        return {'no_upstream_status':r['status'],'unsupported_effort_normalized':'low'}
    finally:
        v.api(f'admin/accounts/{v.account_id}/schedulable',{'schedulable':True},'POST')
        v.reset_account()


def redis_outage():
    dynamic()
    v.control(jev='ok',upstream='ok',jev_model='gpt-6-luna',jev_effort='low')
    first=p.gateway('responses',p.auto_responses('before Redis outage'))
    assert first['status']==200
    before=calls()
    try:
        v.dc('stop','redis')
        r=p.gateway('responses',p.auto_responses('continue',previous_response_id=first['data']['id']),timeout=45)
        assert r['status'] in (409,500,503),r['body'][:400]
        assert calls()==before
        result={'status':r['status'],'error':r.get('data'),'jev_calls':0}
    finally:
        v.dc('up','-d','--wait','redis')
    return result

def main():
    try:
        if not p.check('setup',setup):return 1
        for name,fn in [('same_session_agent_workflow',workflow),('hard_state_and_missing_binding',hard_state),('revocation_and_underlying_permissions',permissions),('streaming_tools',p.endpoints_streaming_tools),('concurrent_session',concurrent_session),('classifier_failures_and_upstream_failures',p.failures),('cancellation',p.client_cancellation),('performance_100_turns_each',performance),('no_upstream_and_unsupported_effort',no_upstream_and_effort),('redis_unavailable',redis_outage),('admin_and_customer_exports',exports)]:p.check(name,fn)
    finally:
        if v.token:
            dynamic(False)
            p.live_config(live_enabled=False,internal_only=True)
            if p.state:q.group(gpt_auto_enabled=False,model_pricing=[],model_allowlist={'enabled':False,'models':[]})
        v.control(jev='ok',upstream='ok',stream_delay=0,input_tokens=None,output_tokens=None,cached=0)
        (OUT/'results.json').write_text(json.dumps(p.results,indent=2))
        summary={'scenarios':len(p.results),'passed':sum(x['passed'] for x in p.results),'failed':[x['name'] for x in p.results if not x['passed']]}
        print(json.dumps(summary),flush=True)
    return bool(summary['failed'])
if __name__=='__main__':raise SystemExit(main())
