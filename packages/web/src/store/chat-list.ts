import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import { createSession, streamChat, type StreamEvent } from '@/composables/api/useChat'

// ---- Message model ----

export interface TextBlock {
  type: 'text'
  content: string
}

export interface ThinkingBlock {
  type: 'thinking'
  content: string
  done: boolean
}

export interface ToolCallBlock {
  type: 'tool_call'
  toolName: string
  input: unknown
  result: unknown | null
  done: boolean
}

export type ContentBlock = TextBlock | ThinkingBlock | ToolCallBlock

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  blocks: ContentBlock[]
  timestamp: Date
  streaming: boolean
}

// ---- Storage helpers ----

const STORAGE_PREFIX = 'chat:'

interface PersistedChat {
  sessionId: string | null
  messages: Array<Omit<ChatMessage, 'timestamp'> & { timestamp: string }>
}

function saveChat(botId: string, sid: string | null, msgs: ChatMessage[]) {
  const key = STORAGE_PREFIX + botId
  const data: PersistedChat = {
    sessionId: sid,
    messages: msgs.map(m => ({
      ...m,
      streaming: false,
      timestamp: m.timestamp.toISOString(),
      // 深拷贝 blocks，避免序列化 reactive proxy 问题
      blocks: JSON.parse(JSON.stringify(m.blocks)),
    })),
  }
  try {
    localStorage.setItem(key, JSON.stringify(data))
  } catch {
    // localStorage 已满，静默忽略
  }
}

function loadChat(botId: string): { sessionId: string | null; messages: ChatMessage[] } | null {
  const key = STORAGE_PREFIX + botId
  const raw = localStorage.getItem(key)
  if (!raw) return null
  try {
    const data: PersistedChat = JSON.parse(raw)
    return {
      sessionId: data.sessionId ?? null,
      messages: data.messages.map(m => ({
        ...m,
        timestamp: new Date(m.timestamp),
        streaming: false,
      })),
    }
  } catch {
    localStorage.removeItem(key)
    return null
  }
}

function removeChat(botId: string) {
  localStorage.removeItem(STORAGE_PREFIX + botId)
}

// ---- Store ----

export const useChatStore = defineStore('chat', () => {
  const messages = reactive<ChatMessage[]>([])
  const streaming = ref(false)
  const currentBotId = ref<string | null>(null)
  const sessionId = ref<string | null>(null)

  let abortFn: (() => void) | null = null

  const nextId = () => `${Date.now()}-${Math.floor(Math.random() * 1000)}`

  /** 持久化当前会话到 localStorage */
  function persist() {
    if (!currentBotId.value) return
    saveChat(currentBotId.value, sessionId.value, messages as ChatMessage[])
  }

  // 切换 Bot
  function selectBot(botId: string) {
    if (currentBotId.value === botId) return
    abort()
    // 保存当前会话
    persist()
    currentBotId.value = botId
    // 尝试从 localStorage 恢复
    const cached = loadChat(botId)
    messages.length = 0
    if (cached) {
      sessionId.value = cached.sessionId
      for (const msg of cached.messages) {
        messages.push(msg)
      }
    } else {
      sessionId.value = null
    }
  }

  // 确保 session 存在
  async function ensureSession() {
    if (!currentBotId.value) throw new Error('No bot selected')
    if (!sessionId.value) {
      sessionId.value = await createSession(currentBotId.value)
    }
  }

  // 中止当前流
  function abort() {
    abortFn?.()
    abortFn = null
    // 标记所有正在流式的消息为完成
    for (const msg of messages) {
      if (msg.streaming) msg.streaming = false
    }
    streaming.value = false
  }

  // 发送消息
  async function sendMessage(text: string) {
    const trimmed = text.trim()
    if (!trimmed || streaming.value || !currentBotId.value) return

    // 添加用户消息
    messages.push({
      id: nextId(),
      role: 'user',
      blocks: [{ type: 'text', content: trimmed }],
      timestamp: new Date(),
      streaming: false,
    })

    streaming.value = true

    try {
      await ensureSession()

      // 创建助手消息占位
      messages.push({
        id: nextId(),
        role: 'assistant',
        blocks: [],
        timestamp: new Date(),
        streaming: true,
      })
      // 从 reactive 数组中获取 proxy 引用，确保后续修改触发响应式
      const assistantMsg = messages[messages.length - 1]!

      // 当前活跃 block 的索引（通过索引访问 reactive proxy，避免引用原始对象）
      let textBlockIdx = -1
      let thinkingBlockIdx = -1

      function pushBlock(block: ContentBlock): number {
        assistantMsg.blocks.push(block)
        return assistantMsg.blocks.length - 1
      }

      abortFn = streamChat(
        currentBotId.value!,
        sessionId.value!,
        trimmed,
        // onEvent
        (event: StreamEvent) => {
          const type = event.type

          switch (type) {
            case 'text_start':
              textBlockIdx = pushBlock({ type: 'text', content: '' })
              break

            case 'text_delta':
              if (typeof event.delta === 'string') {
                if (textBlockIdx < 0 || assistantMsg.blocks[textBlockIdx]?.type !== 'text') {
                  textBlockIdx = pushBlock({ type: 'text', content: '' })
                }
                ;(assistantMsg.blocks[textBlockIdx] as TextBlock).content += event.delta
              }
              break

            case 'text_end':
              textBlockIdx = -1
              break

            case 'reasoning_start':
              thinkingBlockIdx = pushBlock({ type: 'thinking', content: '', done: false })
              break

            case 'reasoning_delta':
              if (typeof event.delta === 'string') {
                if (thinkingBlockIdx < 0 || assistantMsg.blocks[thinkingBlockIdx]?.type !== 'thinking') {
                  thinkingBlockIdx = pushBlock({ type: 'thinking', content: '', done: false })
                }
                ;(assistantMsg.blocks[thinkingBlockIdx] as ThinkingBlock).content += event.delta
              }
              break

            case 'reasoning_end':
              if (thinkingBlockIdx >= 0 && assistantMsg.blocks[thinkingBlockIdx]?.type === 'thinking') {
                ;(assistantMsg.blocks[thinkingBlockIdx] as ThinkingBlock).done = true
              }
              thinkingBlockIdx = -1
              break

            case 'tool_call_start': {
              pushBlock({
                type: 'tool_call',
                toolName: (event.toolName as string) ?? 'unknown',
                input: event.input ?? null,
                result: null,
                done: false,
              })
              textBlockIdx = -1 // tool call 中断文本流
              break
            }

            case 'tool_call_end': {
              // 从头部找第一个未完成的同名 tool_call block
              for (let i = 0; i < assistantMsg.blocks.length; i++) {
                const b = assistantMsg.blocks[i]
                if (b && b.type === 'tool_call' && b.toolName === event.toolName && !b.done) {
                  b.result = event.result ?? null
                  b.done = true
                  break
                }
              }
              break
            }

            case 'agent_start':
            case 'agent_end':
              break

            default: {
              // 兜底：尝试提取文本
              const text = extractFallbackText(event)
              if (text) {
                if (textBlockIdx < 0 || assistantMsg.blocks[textBlockIdx]?.type !== 'text') {
                  textBlockIdx = pushBlock({ type: 'text', content: '' })
                }
                ;(assistantMsg.blocks[textBlockIdx] as TextBlock).content += text
              }
              break
            }
          }
        },
        // onDone
        () => {
          assistantMsg.streaming = false
          streaming.value = false
          abortFn = null
          persist()
        },
        // onError
        () => {
          assistantMsg.streaming = false
          streaming.value = false
          abortFn = null
          persist()
        },
      )
    } catch {
      streaming.value = false
    }
  }

  function clearMessages() {
    abort()
    if (currentBotId.value) removeChat(currentBotId.value)
    messages.length = 0
    sessionId.value = null
  }

  return {
    messages,
    streaming,
    currentBotId,
    selectBot,
    sendMessage,
    clearMessages,
    abort,
  }
})

function extractFallbackText(event: StreamEvent): string | null {
  if (typeof event.delta === 'string') return event.delta
  if (typeof event.text === 'string') return event.text
  if (typeof event.content === 'string') return event.content
  return null
}
