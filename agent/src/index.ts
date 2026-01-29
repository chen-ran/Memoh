import { Elysia } from 'elysia'
import { chatModule } from './modules/chat'
import { corsMiddleware } from './middlewares/cors'
import { errorMiddleware } from './middlewares/error'
import { loadConfig } from './config'

const config = loadConfig('../config.toml')

const app = new Elysia()
  .use(corsMiddleware)
  .use(errorMiddleware)
  .use(chatModule)
  .listen({
    port: config.agent_gateway.port ?? 8081,
    hostname: config.agent_gateway.host ?? '127.0.0.1',
  })

console.log(
  `Agent Gateway is running at ${app.server?.hostname}:${app.server?.port}`
)
