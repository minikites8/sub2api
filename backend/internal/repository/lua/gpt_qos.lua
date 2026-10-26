-- Hierarchical weighted fair queuing, serialized with the existing account
-- semaphore. Redis TIME is the only clock. One call grants at most one turn.
redis.replicate_commands()
local prefix=string.sub(KEYS[1],1,#KEYS[1]-8)
local namespace=string.sub(prefix,1,#prefix-#'gptqos:{v1}:')
local requests=prefix..'requests'
local waiting=prefix..'waiting'
local leases=prefix..'leases'
local closed=prefix..'closed'
local fair=prefix..'fair'
local pools=prefix..'pools'
local op=ARGV[1]
local id=ARGV[2]
local ttl=tonumber(ARGV[3])
local tm=redis.call('TIME')
local now=tonumber(tm[1])*1000+math.floor(tonumber(tm[2])/1000)
local function decode(raw) if raw then return cjson.decode(raw) end return nil end
local function userkey(r) return prefix..'user:'..r.tier..':'..r.user_id end
local function users(r) return prefix..'users:'..r.tier end
local function poolkey(c) return prefix..'pool:'..c.pool_id end
local function groupkey(r) return prefix..'group:'..r.group_id end
local function accountkey(c) return namespace..'concurrency:account:'..c.slot_id end
local function unqueue(r)
 redis.call('ZREM',waiting,r.id)
 redis.call('ZREM',prefix..'tierwait:'..r.tier,r.id)
 redis.call('ZREM',prefix..'groupwait:'..r.group_id,r.id)
 redis.call('ZREM',prefix..'userwait:'..r.user_id,r.id)
 redis.call('ZREM',userkey(r),r.id)
 if redis.call('ZCARD',userkey(r))==0 then redis.call('ZREM',users(r),tostring(r.user_id)) end
end
local function close(rid)
 local r=decode(redis.call('HGET',requests,rid))
 if r then
  unqueue(r)
  if r.grant then
   redis.call('ZREM',accountkey(r.grant.candidate),rid)
   redis.call('ZREM',poolkey(r.grant.candidate),rid)
   redis.call('ZREM',prefix..'strong:'..r.grant.candidate.pool_id,rid)
   redis.call('ZREM',prefix..'all_strong',rid)
   redis.call('ZREM',prefix..'all_strong_premium',rid)
   redis.call('ZREM',prefix..'pool_tier:'..r.grant.candidate.pool_id..':'..r.tier,rid)
   redis.call('ZREM',groupkey(r),rid)
   redis.call('ZREM',leases,rid)
  end
  redis.call('HDEL',requests,rid)
 end
 redis.call('ZADD',closed,now+120000,rid)
 redis.call('PEXPIRE',closed,180000)
end
-- Bounded orphan cleanup, including grants whose owner process disappeared.
for _,rid in ipairs(redis.call('ZRANGEBYSCORE',waiting,'-inf',now,'LIMIT',0,32)) do close(rid) end
for _,rid in ipairs(redis.call('ZRANGEBYSCORE',leases,'-inf',now,'LIMIT',0,32)) do close(rid) end
redis.call('ZREMRANGEBYSCORE',closed,'-inf',now)
-- Bound historical pool metadata after account deletion/pool changes. A small
-- incremental scan avoids an unbounded full registry walk on request admission.
local scan=redis.call('HSCAN',pools,redis.call('HGET',fair,'pool_scan') or '0','COUNT',16)
redis.call('HSET',fair,'pool_scan',scan[1])
for i=1,#scan[2],2 do
 local pid=scan[2][i];local p=decode(scan[2][i+1])
 if now-p.updated>180000 and redis.call('ZCOUNT',prefix..'pool:'..pid,'('..now,'+inf')==0 then
  redis.call('HDEL',pools,pid)
  redis.call('DEL',prefix..'pool:'..pid,prefix..'strong:'..pid,prefix..'pool_tier:'..pid..':premium',prefix..'pool_tier:'..pid..':standard',prefix..'pool_tier:'..pid..':economy')
 end
end
if op=='close' then close(id);return '{}' end
local own=decode(redis.call('HGET',requests,id))
if op=='refresh' then
 if not own or not own.grant or not redis.call('ZSCORE',leases,id) or not redis.call('ZSCORE',accountkey(own.grant.candidate),id) then return 'false' end
 redis.call('ZADD',leases,now+ttl*1000,id)
 redis.call('ZADD',poolkey(own.grant.candidate),now+ttl*1000,id)
 redis.call('ZADD',prefix..'pool_tier:'..own.grant.candidate.pool_id..':'..own.tier,now+ttl*1000,id)
 if own.grant.candidate.quality=='strong' then redis.call('ZADD',prefix..'strong:'..own.grant.candidate.pool_id,now+ttl*1000,id);redis.call('ZADD',prefix..'all_strong',now+ttl*1000,id) end
 if own.grant.candidate.quality=='strong' and own.tier=='premium' and own.protected then redis.call('ZADD',prefix..'all_strong_premium',now+ttl*1000,id) end
 redis.call('ZADD',groupkey(own),now+ttl*1000,id)
 local pm=decode(redis.call('HGET',pools,own.grant.candidate.pool_id));if pm then pm.updated=now;redis.call('HSET',pools,own.grant.candidate.pool_id,cjson.encode(pm)) end
 redis.call('ZADD',accountkey(own.grant.candidate),math.floor(now/1000),id)
 redis.call('EXPIRE',accountkey(own.grant.candidate),ttl)
 redis.call('ZADD',namespace..'concurrency:account:active_index',math.floor(now/1000)+ttl,tostring(own.grant.candidate.slot_id))
 return 'true'
end
if op=='state' then
 local out={queued=redis.call('ZCOUNT',waiting,'('..now,'+inf'),active=redis.call('ZCOUNT',leases,'('..now,'+inf'),pools={},queued_by_tier={}}
 out.strong_pool=decode(redis.call('GET',prefix..'global_peak')) or {}
 out.strong_pool.active=redis.call('ZCOUNT',prefix..'all_strong','('..now,'+inf')
 out.strong_pool.premium_active=redis.call('ZCOUNT',prefix..'all_strong_premium','('..now,'+inf')
 out.strong_pool.premium_reserved_used=math.min(out.strong_pool.premium_active,out.strong_pool.reserved or 0)
 for _,tier in ipairs({'premium','standard','economy'}) do out.queued_by_tier[tier]=redis.call('ZCOUNT',prefix..'tierwait:'..tier,'('..now,'+inf') end
 local all=redis.call('HGETALL',pools)
 for i=1,#all,2 do
  local p=decode(all[i+1])
  if now-p.updated<120000 then
   p.active=redis.call('ZCOUNT',prefix..'pool:'..all[i],'('..now,'+inf');p.id=all[i]
   p.strong_active=redis.call('ZCOUNT',prefix..'strong:'..all[i],'('..now,'+inf')
   p.premium_active=redis.call('ZCOUNT',prefix..'pool_tier:'..all[i]..':premium','('..now,'+inf')
   p.premium_reserved_used=math.min(p.premium_active,p.reserved or 0)
   table.insert(out.pools,p)
  end
 end
 return cjson.encode(out)
end
if own and own.grant then return cjson.encode(own.grant) end
if redis.call('ZSCORE',closed,id) then return '{"status":"closed"}' end
if not own then
 own=cjson.decode(ARGV[4])
 if redis.call('ZCARD',waiting)>=own.MaxWaiting or redis.call('ZCARD',prefix..'groupwait:'..own.group_id)>=own.GroupMaxWaiting or redis.call('ZCARD',prefix..'userwait:'..own.user_id)>=own.UserMaxWaiting then return '{"status":"full"}' end
 own.created=now;own.deadline=now+own.WaitMs;own.strong_after=now+own.StrongWaitMs
 redis.call('ZADD',waiting,own.deadline,id)
 redis.call('ZADD',prefix..'tierwait:'..own.tier,own.deadline,id)
 redis.call('ZADD',prefix..'groupwait:'..own.group_id,own.deadline,id)
 redis.call('ZADD',prefix..'userwait:'..own.user_id,own.deadline,id)
 if redis.call('ZCARD',users(own))==0 then redis.call('HSET',fair,'tier:'..own.tier,redis.call('HGET',fair,'virtual') or '0') end
 if not redis.call('ZSCORE',users(own),tostring(own.user_id)) then redis.call('ZADD',users(own),tonumber(redis.call('HGET',fair,'user:'..own.tier) or '0'),tostring(own.user_id)) end
 local seq=redis.call('HINCRBY',fair,'sequence',1)
 redis.call('ZADD',userkey(own),seq,id)
 redis.call('PEXPIRE',userkey(own),own.WaitMs+180000)
 redis.call('PEXPIRE',prefix..'groupwait:'..own.group_id,own.WaitMs+180000)
 redis.call('PEXPIRE',prefix..'userwait:'..own.user_id,own.WaitMs+180000)
 redis.call('HSET',requests,id,cjson.encode(own))
 for _,c in ipairs(own.candidates) do
  -- A pool ID is an explicit deduplication boundary. Shared keys never add
  -- their declared caps together; every account uses the same configured cap.
  local p=decode(redis.call('HGET',pools,c.pool_id)) or {capacity=c.pool_capacity,peak=false,since=0,updated=now}
  p.capacity=c.pool_capacity;p.updated=now
  redis.call('HSET',pools,c.pool_id,cjson.encode(p))
 end
end
local function available(r)
 if r.group_limit>0 then
  redis.call('ZREMRANGEBYSCORE',groupkey(r),'-inf',now)
  if redis.call('ZCARD',groupkey(r))>=r.group_limit then return nil end
 end
 -- Aggregate protection covers many one-slot independent accounts as well as
 -- shared upstream roots. Strong capacities are deduplicated by the registry.
 local globalCapacity=tonumber(redis.call('HGET',prefix..'capacity_policy','strong_capacity') or '0')
 local gp=decode(redis.call('GET',prefix..'global_peak')) or {peak=false,since=0}
 local globalBlocked=false
 if r.peak.enabled and globalCapacity>0 then
  redis.call('ZREMRANGEBYSCORE',prefix..'all_strong','-inf',now)
  local globalUsed=redis.call('ZCARD',prefix..'all_strong')
  local utilization=globalUsed/globalCapacity
  local crossed=(not gp.peak and utilization>=r.peak.enter_utilization) or (gp.peak and utilization<=r.peak.exit_utilization)
  if crossed then
   if gp.since==0 then gp.since=now end
   if now-gp.since>=(gp.peak and r.peak.exit_hold_ms or r.peak.enter_hold_ms) then gp.peak=not gp.peak;gp.since=0 end
  else gp.since=0 end
  local reserve=0
  if gp.peak and globalCapacity>=2 and r.peak.protection_ratio>0 then reserve=math.min(globalCapacity-1,math.max(1,math.floor(globalCapacity*r.peak.protection_ratio))) end
  gp.capacity=globalCapacity;gp.active=globalUsed;gp.reserved=reserve;gp.borrowing=not r.peak.strict_reserve;gp.updated=now
  gp.reason=gp.peak and 'aggregate_strong_utilization_above_threshold_stable' or 'below_threshold_or_hysteresis_hold'
  redis.call('SET',prefix..'global_peak',cjson.encode(gp))
  globalBlocked=not (r.tier=='premium' and r.protected) and reserve>0 and (r.peak.strict_reserve or redis.call('ZCARD',prefix..'users:premium')>0) and globalUsed>=globalCapacity-reserve
 end
 local choice=nil
 for _,c in ipairs(r.candidates) do
  if c.quality~='basic' or now>=r.strong_after then
   local ak=accountkey(c);local lk=namespace..'concurrency:live:account:'..c.slot_id
   redis.call('ZREMRANGEBYSCORE',ak,'-inf',math.floor(now/1000)-ttl)
   redis.call('ZREMRANGEBYSCORE',lk,'-inf',math.floor(now/1000)-60)
   local accountUsed=redis.call('ZCARD',ak)+redis.call('ZCARD',lk)
   local pk=poolkey(c)
   redis.call('ZREMRANGEBYSCORE',pk,'-inf',now)
   local used=redis.call('ZCARD',pk)
   local configured=tonumber(redis.call('HGET',prefix..'pool_limits',c.pool_id) or tostring(c.pool_capacity))
   local effectiveCapacity=math.min(c.pool_capacity,configured)
   -- An independent pool includes non-QoS/Live occupancy in the same existing
   -- account semaphore. Shared pool admission is additionally capped by pk.
   if string.sub(c.pool_id,1,8)=='account:' then used=math.max(used,accountUsed) end
   local strongKey=prefix..'strong:'..c.pool_id
   redis.call('ZREMRANGEBYSCORE',strongKey,'-inf',now)
   local strongUsed=redis.call('ZCARD',strongKey)
   local strongCapacity=tonumber(redis.call('HGET',prefix..'strong_limits',c.pool_id) or tostring(effectiveCapacity))
   if string.sub(c.pool_id,1,8)=='account:' then strongUsed=used end
   local p=decode(redis.call('HGET',pools,c.pool_id)) or {capacity=c.pool_capacity,peak=false,since=0,updated=now}
   local peak=r.peak
   local protected=false;local reserve=0
   if not peak.enabled then p.peak=false;p.since=0;p.reserved=0 end
   if strongCapacity>0 and peak.enabled then
    local utilization=strongUsed/math.max(1,strongCapacity)
    if not p.peak then
     if utilization>=peak.enter_utilization then
      if p.since==0 then p.since=now end
      if now-p.since>=peak.enter_hold_ms then p.peak=true;p.since=0 end
     else p.since=0 end
    else
     if utilization<=peak.exit_utilization then
      if p.since==0 then p.since=now end
      if now-p.since>=peak.exit_hold_ms then p.peak=false;p.since=0 end
     else p.since=0 end
    end
    if p.peak and strongCapacity>=2 and peak.protection_ratio>0 then reserve=math.min(strongCapacity-1,math.max(1,math.floor(strongCapacity*peak.protection_ratio))) end
    local premium=(r.tier=='premium' and r.protected)
    protected=not premium and reserve>0 and (peak.strict_reserve or redis.call('ZCARD',prefix..'users:premium')>0) and (used>=effectiveCapacity-reserve or (c.quality=='strong' and strongUsed>=strongCapacity-reserve))
   end
   p.updated=now;p.capacity=effectiveCapacity;p.strong_capacity=strongCapacity;p.strong_active=strongUsed;p.reserved=reserve;p.reason=p.peak and 'strong_utilization_above_threshold_stable' or 'below_threshold_or_hysteresis_hold';p.borrowing=(not peak.strict_reserve);redis.call('HSET',pools,c.pool_id,cjson.encode(p))
   if accountUsed<c.concurrency and used<effectiveCapacity and not protected and not (c.quality=='strong' and globalBlocked) then
    -- Candidate order contains soft cost/protocol preferences, not a capacity
    -- mandate. Compare live Redis occupancy inside the atomic grant instead
    -- of sending every short request to the first account with spare slots.
    -- Shared roots use one load and one idle-turn history regardless of how
    -- many API keys point at the same physical resource.
    local quality=c.quality=='strong' and 0 or (c.quality=='basic' and 1 or 2)
    local standby=c.is_fallback and 1 or 0
    local load=math.max(accountUsed/c.concurrency,used/effectiveCapacity)
    local last=p.last_dispatch or 0
    if not choice or quality<choice.quality or (quality==choice.quality and (standby<choice.standby or (standby==choice.standby and (load<choice.load or (load==choice.load and last<choice.last))))) then
     choice={candidate=c,quality=quality,standby=standby,load=load,last=last,peak=p.peak or (r.peak.enabled and gp.peak),reserve=reserve}
    end
   end
  end
 end
 if choice then return choice.candidate,choice.peak,choice.reserve end
 return nil
end
local best=nil
for ti,tier in ipairs({'premium','standard','economy'}) do
 local uk=prefix..'users:'..tier
 -- Bounded user scan avoids a slow model blocking an unrelated user's work.
 local count=redis.call('ZCARD',uk)
 local offset=tonumber(redis.call('HGET',fair,'scan:'..tier) or '0');if offset>=count then offset=0 end
 local found=false
 for _,uid in ipairs(redis.call('ZRANGE',uk,offset,offset+63)) do
  local q=prefix..'user:'..tier..':'..uid
  local heads=redis.call('ZRANGE',q,0,0)
  if #heads>0 then
   local r=decode(redis.call('HGET',requests,heads[1]))
   if r and not r.grant and r.deadline>now then
    local candidate,peak,reserve=available(r)
    if candidate then
     found=true
     local defaults={6,3,1}
     local weight=r.weights[ti]*r.weight/defaults[ti]
     local finish=tonumber(redis.call('HGET',fair,'tier:'..tier) or '0')+1/weight
     if not best or finish<best.finish then best={r=r,candidate=candidate,peak=peak,reserve=reserve,finish=finish,userScore=tonumber(redis.call('ZSCORE',uk,uid) or '0'),userWeight=r.weight/defaults[ti]} end
     break
    end
   end
  end
 end
 if not found and count>64 then redis.call('HSET',fair,'scan:'..tier,tostring((offset+64)%count)) else redis.call('HSET',fair,'scan:'..tier,'0') end
end
if best then
 local r=best.r;local c=best.candidate
 r.grant={status='granted',candidate=c,peak=best.peak,reserved=best.reserve,wait_ms=now-r.created}
 r.candidates=nil -- no longer needed after atomic assignment; keep grant records small
 local encoded=cjson.encode(r)
 if type(encoded)~='string' then return redis.error_reply('QoS grant encoding failed') end
 -- All checks and all capacity claims occur in this same Redis transaction.
 redis.call('ZADD',accountkey(c),math.floor(now/1000),r.id)
 redis.call('EXPIRE',accountkey(c),ttl)
 redis.call('ZADD',namespace..'concurrency:account:active_index',math.floor(now/1000)+ttl,tostring(c.slot_id))
 redis.call('ZADD',poolkey(c),now+ttl*1000,r.id)
 redis.call('ZADD',prefix..'pool_tier:'..c.pool_id..':'..r.tier,now+ttl*1000,r.id)
 if c.quality=='strong' then redis.call('ZADD',prefix..'strong:'..c.pool_id,now+ttl*1000,r.id);redis.call('ZADD',prefix..'all_strong',now+ttl*1000,r.id) end
 if c.quality=='strong' and r.tier=='premium' and r.protected then redis.call('ZADD',prefix..'all_strong_premium',now+ttl*1000,r.id) end
 redis.call('ZADD',groupkey(r),now+ttl*1000,r.id)
 redis.call('ZADD',leases,now+ttl*1000,r.id)
 local selectedPool=decode(redis.call('HGET',pools,c.pool_id))
 selectedPool.last_dispatch=redis.call('HINCRBY',fair,'dispatch_sequence',1)
 redis.call('HSET',pools,c.pool_id,cjson.encode(selectedPool))
 unqueue(r)
 if redis.call('ZCARD',userkey(r))>0 then redis.call('ZADD',users(r),tostring(best.userScore+1/best.userWeight),tostring(r.user_id)) end
 redis.call('HSET',fair,'tier:'..r.tier,tostring(best.finish),'virtual',tostring(best.finish),'user:'..r.tier,tostring(best.userScore))
 redis.call('HSET',requests,r.id,encoded)
 if r.id==id then return cjson.encode(r.grant) end
end
return '{"status":"waiting"}'
