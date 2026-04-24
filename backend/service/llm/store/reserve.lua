-- KEYS[1] = ratelimit:{slotID}:minute          (ZSET)
-- KEYS[2] = ratelimit:{slotID}:day:{UTCDATE}   (INT)
-- KEYS[3] = ratelimit:{slotID}:tpm:window      (ZSET)
-- KEYS[4] = ratelimit:{slotID}:tpm:tokens      (HASH)
-- KEYS[5] = ratelimit:{slotID}:tpm:sum         (INT)
-- ARGV[1] = now_millis
-- ARGV[2] = rpm_limit        (0 = unlimited)
-- ARGV[3] = rpd_limit        (0 = unlimited)
-- ARGV[4] = tpm_limit        (0 = unlimited)
-- ARGV[5] = tokens           (estimated input tokens for this call)
-- ARGV[6] = reqID
-- ARGV[7] = day_expire_millis (end-of-UTC-day + grace)
-- Returns: 0 OK | 1 RPM | 2 RPD | 3 TPM

local now    = tonumber(ARGV[1])
local rpm    = tonumber(ARGV[2])
local rpd    = tonumber(ARGV[3])
local tpm    = tonumber(ARGV[4])
local tokens = tonumber(ARGV[5])
local req    = ARGV[6]
local exp    = tonumber(ARGV[7])

-- Evict minute + tpm entries older than 60s.
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - 60000)
local evicted = redis.call('ZRANGEBYSCORE', KEYS[3], '-inf', now - 60000)
if #evicted > 0 then
    for _, rid in ipairs(evicted) do
        local t = redis.call('HGET', KEYS[4], rid)
        if t then
            redis.call('DECRBY', KEYS[5], tonumber(t))
            redis.call('HDEL', KEYS[4], rid)
        end
    end
    redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', now - 60000)
end

-- Ensure sum never goes negative (defensive for crash recovery).
local sum = tonumber(redis.call('GET', KEYS[5]) or '0')
if sum < 0 then
    redis.call('SET', KEYS[5], 0)
    sum = 0
end

if rpm > 0 then
    local n = redis.call('ZCARD', KEYS[1])
    if n >= rpm then return 1 end
end

if rpd > 0 then
    local d = tonumber(redis.call('GET', KEYS[2]) or '0')
    if d >= rpd then return 2 end
end

if tpm > 0 and (sum + tokens) > tpm then
    return 3
end

redis.call('ZADD', KEYS[1], now, req)
redis.call('EXPIRE', KEYS[1], 120)

if tokens > 0 then
    redis.call('ZADD', KEYS[3], now, req)
    redis.call('HSET', KEYS[4], req, tokens)
    redis.call('INCRBY', KEYS[5], tokens)
    redis.call('EXPIRE', KEYS[3], 120)
    redis.call('EXPIRE', KEYS[4], 120)
    redis.call('EXPIRE', KEYS[5], 120)
end

redis.call('INCR', KEYS[2])
redis.call('PEXPIREAT', KEYS[2], exp)
return 0
