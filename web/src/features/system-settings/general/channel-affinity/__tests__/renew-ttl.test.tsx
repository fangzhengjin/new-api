import { describe, expect, test } from 'vitest'

import {
  routingPolicyFormValues,
  routingPolicyOptions,
} from '../../../request-policies/routing-form'

// renew_ttl_on_success 在受控架构下是 settings 键：表单反序列化必须携带该值，
// 否则运维无法查看或切换亲和 TTL 续期行为（默认 true，service/channel_affinity.go 消费）。
describe('renew ttl setting roundtrip', () => {
  test('deserializes renew_ttl_on_success from option values', () => {
    const values = routingPolicyFormValues({
      RetryTimes: '0',
      AutomaticRetryStatusCodes: '',
      'channel_affinity_setting.enabled': 'true',
      'channel_affinity_setting.renew_ttl_on_success': 'false',
      'channel_affinity_setting.switch_on_success': 'true',
      'channel_affinity_setting.keep_on_channel_disabled': 'false',
      'channel_affinity_setting.max_entries': '100000',
      'channel_affinity_setting.default_ttl_seconds': '3600',
      'channel_affinity_setting.rules': '[]',
    })
    expect(values.channel_affinity_setting.renew_ttl_on_success).toBe(false)
  })
})

test('serializes renew_ttl_on_success back into option keys', () => {
  const values = routingPolicyFormValues({
    RetryTimes: '0',
    AutomaticRetryStatusCodes: '',
    'channel_affinity_setting.enabled': 'true',
    'channel_affinity_setting.renew_ttl_on_success': 'false',
    'channel_affinity_setting.switch_on_success': 'true',
    'channel_affinity_setting.keep_on_channel_disabled': 'false',
    'channel_affinity_setting.max_entries': '100000',
    'channel_affinity_setting.default_ttl_seconds': '3600',
    'channel_affinity_setting.rules': '[]',
  })
  const options = routingPolicyOptions(values)
  expect(options['channel_affinity_setting.renew_ttl_on_success']).toBe('false')
})
