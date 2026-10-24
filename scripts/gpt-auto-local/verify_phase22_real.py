"""Opt-in real Jev smoke test. Synthetic prompts only; local mock executes models.
Set GPT_AUTO_TEST_JEV_KEY in the process environment. Never write credentials to evidence.
"""
import json
import os
import uuid
import verify_phase22 as d
p,v,q=d.p,d.v,d.q

def main():
    key=os.environ.pop('GPT_AUTO_TEST_JEV_KEY','').strip()
    if not key:raise RuntimeError('GPT_AUTO_TEST_JEV_KEY is required')
    rows=[]
    try:
        d.setup()
        cfg=v.api('admin/gpt-auto/config')
        cfg.update(classifier_base_url='https://apinode.ltd',classifier_api_key=key,classifier_model='jev-latest',classifier_proxy_id=None,classifier_timeout_ms=30000,classifier_retry_count=0)
        cfg['live'].update(classifier_timeout_ms=30000,classifier_failure_policy='reject_request')
        v.api('admin/gpt-auto/config',cfg,'PUT')
        del key
        session='phase22-real-'+uuid.uuid4().hex
        texts=[
          'Implement a small helper that trims whitespace and returns a default label when the string is empty. Only one pure function is needed.',
          'The helper task is done. Diagnose this new distributed worker deadlock: the coordinator holds a metadata lock while waiting for worker acknowledgement; a worker holds the queue lock while waiting for metadata refresh. A lease expires concurrently and another worker may repeat a non-idempotent external action. Establish the wait-for cycle, design a lock ordering and fencing protocol, and reason about cancellation, lease renewal and crash recovery without duplicating the external action.',
          'The concurrency repair has been implemented and verified. Only rewrite this documentation sentence for clarity: The helper returns the supplied fallback if the trimmed label is empty. Do not revisit the concurrency design.',
          'The concurrency repair has been implemented and verified. Only rewrite this documentation sentence for clarity: The helper returns the supplied fallback if the trimmed label is empty. Do not revisit the concurrency design.',
        ]
        history=[]
        for i,text in enumerate(texts):
            history.append({'role':'user','content':text})
            r=p.gateway('chat/completions',{'model':'gpt-auto','messages':history},headers={'session-id':session},timeout=45)
            row={'turn':i+1,'http_status':r['status'],'request_ms':r['ms']}
            if r['status']==200:
                detail=d.detail(r);t=detail['features']['switching'];usage=p.wait_usage(p.rid(r))
                row.update(jev_model=detail['jev_model'],jev_effort=detail['jev_effort'],executed_model=detail['final_model'],executed_effort=detail['final_effort'],classification_trigger=t['classification_trigger'],switch_direction=t['switch_direction'],switch_block_reason=t.get('switch_block_reason'),classification_ms=detail['classifier_latency_ms'],estimated_classifier_cost=detail['classifier_cost'],customer_charge=usage[2])
                row['classifier_usage']=v.sql("SELECT classifier_input_tokens,classifier_output_tokens,attempts FROM gpt_auto_shadow_decisions WHERE request_id='client:"+p.rid(r)+"'")
                assert detail['jev_model'] in q.POOL and detail['final_model'] in q.POOL
                assert usage[2]=='0.0057000000',usage
            else:
                row['error']=r.get('data')
            rows.append(row)
            history.append({'role':'assistant','content':'Synthetic previous stage completed; now evaluate only the next instruction.'})
        passed=all(r['http_status']==200 for r in rows)
        evidence={'provider':'https://apinode.ltd','synthetic_only':True,'quality_benchmark':False,'passed':passed,'rows':rows}
        (d.OUT/'real-jev.json').write_text(json.dumps(evidence,indent=2))
        print(json.dumps(evidence),flush=True)
        return not passed
    finally:
        if v.token:
            # Restore the synthetic provider and all local routing switches OFF.
            cfg=v.api('admin/gpt-auto/config')
            cfg.update(classifier_base_url='https://mock:8443',classifier_api_key='synthetic-local-jev-key',classifier_proxy_id=v.proxy_id,classifier_timeout_ms=500)
            cfg['live'].update(live_enabled=False,internal_only=True,classifier_timeout_ms=1500,classifier_failure_policy='compatible_fallback')
            cfg['live']['dynamic_routing']['enabled']=False
            v.api('admin/gpt-auto/config',cfg,'PUT')
            if p.state:q.group(gpt_auto_enabled=False,model_pricing=[])
if __name__=='__main__':raise SystemExit(main())
