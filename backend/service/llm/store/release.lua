-- KEYS[1] = ratelimit:{slotID}:minute          (ZSET)
-- KEYS[2] = ratelimit:{slotID}:day:{UTCDATE}   (INT)
-- KEYS[3] = ratelimit:{slotID}:tpm:window      (ZSET)
-- KEYS[4] = ratelimit:{slotID}:tpm:tokens      (HASH)
-- KEYS[5] = ratelimit:{slotID}:tpm:sum         (INT)
-- ARGV[1] = reqID
--
-- Mirror image of reserve.lua: unwind the per-request bookkeeping a
-- successful Reserve added. Unknown reqIDs are a no-op so the router can
-- safely call Release for any in-flight call without first checking whether
-- it actually reached Reserve.

local req = ARGV[1]

redis.call('ZREM', KEYS[1], req)
redis.call('ZREM', KEYS[3], req)

local tokens = redis.call('HGET', KEYS[4], req)
if tokens then
    local n = tonumber(tokens)
    if n and n > 0 then
        redis.call('DECRBY', KEYS[5], n)
    end
    redis.call('HDEL', KEYS[4], req)
end

redis.call('DECR', KEYS[2])
return 1
