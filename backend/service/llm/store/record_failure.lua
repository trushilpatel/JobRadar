-- KEYS[1] = cooldown:{slotID}:failure_count
-- KEYS[2] = cooldown:{slotID}:cooldown_until
-- ARGV[1] = base_millis
-- ARGV[2] = max_millis
-- ARGV[3] = jitter_multiplier  (e.g. 0.87 = random in 1±0.2 pre-computed in Go)
-- ARGV[4] = now_millis
-- ARGV[5] = count_expire_millis  (set EXPIREAT so the counter eventually drops)
-- Returns: the new until_millis

local base    = tonumber(ARGV[1])
local max_ms  = tonumber(ARGV[2])
local jitter  = tonumber(ARGV[3])
local now     = tonumber(ARGV[4])
local cntExp  = tonumber(ARGV[5])

local n = redis.call('INCR', KEYS[1])

-- Guard against overflow: cap shift at 30 (mirrors computeBackoff in Go).
local shift = n - 1
if shift < 0 then shift = 0 end
if shift > 30 then shift = 30 end

local dur = base * (2 ^ shift)
if dur <= 0 then
    dur = max_ms
end
if dur > max_ms then
    dur = max_ms
end

dur = dur * jitter
if dur <= 0 then
    dur = base
end

local until_ms = now + math.floor(dur)

-- PEXPIREAT needs integer millis.
local untilExpire = until_ms + 3600000

redis.call('SET', KEYS[2], until_ms, 'PX', math.floor(dur) + 3600000)
redis.call('PEXPIREAT', KEYS[1], cntExp)

return until_ms
