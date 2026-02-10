// ---- Types ----

export interface StreamEvent {
  type?:
    | 'text_start' | 'text_delta' | 'text_end'
    | 'reasoning_start' | 'reasoning_delta' | 'reasoning_end'
    | 'tool_call_start' | 'tool_call_end'
    | 'agent_start' | 'agent_end'
  delta?: string
  toolName?: string
  input?: unknown
  result?: unknown
  [key: string]: unknown
}

export type StreamEventHandler = (event: StreamEvent) => void

// ---- Session ----

export async function createSession(botId: string): Promise<string> {
  const token = localStorage.getItem('token') ?? ''
  const resp = await fetch(`/api/bots/${botId}/web/sessions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
  })
  if (!resp.ok) throw new Error(`Failed to create session: ${resp.status}`)
  const data = await resp.json()
  return data.session_id
}

// ---- Streaming chat ----

export function streamChat(
  botId: string,
  sessionId: string,
  query: string,
  onEvent: StreamEventHandler,
  onDone: () => void,
  onError: (err: Error) => void,
): () => void {
  const controller = new AbortController()
  const token = localStorage.getItem('token') ?? ''

  ;(async () => {
    try {
      const resp = await fetch(
        `/api/bots/${botId}/chat/stream?session_id=${encodeURIComponent(sessionId)}`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`,
          },
          body: JSON.stringify({ query }),
          signal: controller.signal,
        },
      )

      if (!resp.ok || !resp.body) {
        onError(new Error(`Chat request failed: ${resp.status}`))
        return
      }

      const reader = resp.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { value, done } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })

        let idx: number
        while ((idx = buffer.indexOf('\n')) >= 0) {
          const line = buffer.slice(0, idx).trim()
          buffer = buffer.slice(idx + 1)
          if (!line.startsWith('data:')) continue
          const payload = line.slice(5).trim()
          if (!payload || payload === '[DONE]') continue

          try {
            const event = JSON.parse(payload) as StreamEvent
            onEvent(event)
          } catch {
            // 非 JSON payload，尝试作为纯文本
            onEvent({ type: 'text_delta', delta: payload })
          }
        }
      }

      onDone()
    } catch (err) {
      if ((err as Error).name !== 'AbortError') {
        onError(err as Error)
      }
    }
  })()

  return () => controller.abort()
}
