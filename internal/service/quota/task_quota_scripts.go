package quota

import "va_visionai_server/conf"

// RedisKeyTTL 统一的 Redis Key TTL（秒）
func RedisKeyTTL() int {
	ttl := 3600
	if conf.GlobalConfig.Reconciler.KeyTTLSeconds > 0 {
		ttl = conf.GlobalConfig.Reconciler.KeyTTLSeconds
	}
	return ttl
}

// LuaCheckAndReserveScript 统一的 Lua 脚本：原子性地检查并预留槽位（队列/并发通用）
const LuaCheckAndReserveScript = `
local key = KEYS[1]
local task_id = ARGV[1]
local max_limit = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])

local current = tonumber(redis.call('GET', key) or '0')
if current >= max_limit then
  return {0, current}
end

redis.call('INCR', key)
redis.call('EXPIRE', key, ttl)
redis.call('SADD', key .. ':tasks', task_id)
redis.call('EXPIRE', key .. ':tasks', ttl)

return {1, current + 1}
`

// LuaReleaseSlotScript 统一的 Lua 脚本：原子性地释放槽位（队列/并发通用）
const LuaReleaseSlotScript = `
local key = KEYS[1]
local task_id = ARGV[1]

-- 验证任务ID是否存在
local exists = redis.call('SISMEMBER', key .. ':tasks', task_id)
if exists == 0 then
  return 0  -- 任务ID不存在，可能已释放
end

-- 移除任务ID
redis.call('SREM', key .. ':tasks', task_id)

-- 减少计数（确保不会小于0）
local current = tonumber(redis.call('GET', key) or '0')
if current > 0 then
  redis.call('DECR', key)
  return 1
end

return 0
`
