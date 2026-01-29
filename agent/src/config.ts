import { readFileSync } from 'fs'
import { parse } from 'toml'

type AgentGatewayConfig = {
  'agent_gateway': {
    host?: string
    port?: number
  }
}

export const loadConfig = (path: string = './config.toml'): AgentGatewayConfig => {
  const config = parse(readFileSync(path, 'utf-8'))
  return config satisfies AgentGatewayConfig
}