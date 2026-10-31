// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref, type App } from 'vue'
import { createPinia } from 'pinia'
import { PiniaColada } from '@pinia/colada'
import type { BotagentsBotAgent } from '@memohai/sdk'

const api = vi.hoisted(() => ({ account: vi.fn(), controls: vi.fn(), authorize: vi.fn() }))
vi.mock('@memohai/sdk', () => ({
  getBotsByBotIdAgentsByIdAccount: api.account,
  getBotsByBotIdAgentsByIdRuntimeControls: api.controls,
  postBotsByBotIdAgentsByIdLoginDeviceAuthorize: api.authorize,
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({
  t: (key: string, params?: { account?: string }) => params?.account ? `${key}: ${params.account}` : key,
  te: () => true,
}) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('./codex-usage-rows.vue', () => ({ default: { template: '<div data-usage />' } }))
vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })

import SettingsDirectAgentDetail from './settings-direct-agent-detail.vue'

const agent = ref<BotagentsBotAgent>({})
let app: App | undefined
let root: HTMLDivElement
const settle = async () => { await new Promise(resolve => setTimeout(resolve, 0)); await nextTick() }
function connectButton() {
  return [...root.querySelectorAll('button')].find(button => button.textContent?.includes('provider.oauth.connect'))!
}
beforeEach(() => {
  vi.clearAllMocks()
  api.controls.mockResolvedValue({ data: {} })
  api.account.mockImplementation(async () => {
    if (!agent.value.enabled) throw { code: 'bot_agent.not_found' }
    return { data: { signed_in: true, email: 'qa@example.test' } }
  })
  root = document.createElement('div')
  document.body.append(root)
})
afterEach(() => { app?.unmount(); app = undefined; document.body.innerHTML = '' })

it.each([['codex', 'chatgpt'], ['grok-build', 'grok_account']])(
  'keeps disabled %s settings usable and resumes account lookup when enabled', async (runtime, auth) => {
    agent.value = { id: 'agent-1', name: 'QA Agent', runtime, enabled: false, metadata: { provider: runtime, auth } }
    app = createApp(() => h(SettingsDirectAgentDetail, { botId: 'bot-1', agent: agent.value }))
    app.use(createPinia()).use(PiniaColada)
    app.config.globalProperties.$t = (key: string) => key
    app.mount(root)
    await settle()

    expect(api.account).not.toHaveBeenCalled()
    expect(api.controls).not.toHaveBeenCalled()
    expect(root.textContent).toContain('bots.agent.enableToSignIn')
    expect(connectButton().disabled).toBe(true)
    connectButton().click()
    expect(api.authorize).not.toHaveBeenCalled()

    agent.value = { ...agent.value, enabled: true }
    await vi.waitFor(() => expect(root.textContent).toContain('qa@example.test'))
    expect(api.account).toHaveBeenCalledOnce()
    expect(api.controls).toHaveBeenCalledOnce()

    agent.value = { ...agent.value, enabled: false }
    await settle()
    expect(root.textContent).toContain('bots.agent.enableToSignIn')
    expect(root.textContent).not.toContain('qa@example.test')
    expect(connectButton().disabled).toBe(true)
    expect(api.account).toHaveBeenCalledOnce()
  },
)
