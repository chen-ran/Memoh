import { createClient, requireAuth } from './client'
import type { Model, ApiResponse } from '../types'

export interface CreateModelParams {
  name: string
  modelId: string
  baseUrl: string
  apiKey: string
  clientType: string
  type?: 'chat' | 'embedding'
  dimensions?: number
}

export interface ModelListItem {
  id: string
  model: Model
}

/**
 * List all models
 */
export async function listModels(): Promise<ModelListItem[]> {
  requireAuth()
  const client = createClient()
  
  const response = await client.model.get()

  if (response.error) {
    throw new Error(response.error.value)
  }

  const data = response.data as { success?: boolean; items?: ModelListItem[] } | null
  if (data?.success && data?.items) {
    return data.items
  }
  
  throw new Error('Failed to fetch model list')
}

/**
 * Create model configuration
 */
export async function createModel(params: CreateModelParams): Promise<Model> {
  requireAuth()
  const client = createClient()

  const payload: Record<string, unknown> = {
    name: params.name,
    modelId: params.modelId,
    baseUrl: params.baseUrl,
    apiKey: params.apiKey,
    clientType: params.clientType,
    type: params.type || 'chat',
  }

  // If embedding type, add dimensions
  if (params.type === 'embedding') {
    if (!params.dimensions) {
      throw new Error('Embedding models require dimensions to be specified')
    }
    payload.dimensions = params.dimensions
  }


  const response = await client.model.post(payload)

  if (response.error) {
    throw new Error(response.error.value)
  }

  const data = response.data as ApiResponse<Model> | null
  if (data?.success && data?.data) {
    return data.data
  }
  
  throw new Error('Failed to create model configuration')
}

/**
 * Get model by ID
 */
export async function getModel(id: string): Promise<Model> {
  requireAuth()
  const client = createClient()

  const response = await client.model({ id }).get()

  if (response.error) {
    throw new Error(response.error.value)
  }

  const data = response.data as ApiResponse<Model> | null
  if (data?.success && data?.data) {
    return data.data
  }
  
  throw new Error('Failed to fetch model configuration')
}

/**
 * Delete model
 */
export async function deleteModel(id: string): Promise<void> {
  requireAuth()
  const client = createClient()

  const response = await client.model({ id }).delete()

  if (response.error) {
    throw new Error(response.error.value)
  }
}

/**
 * Get default models
 */
export async function getDefaultModels(): Promise<{
  chat?: Model
  summary?: Model
  embedding?: Model
}> {
  requireAuth()
  const client = createClient()

  const [chatRes, summaryRes, embeddingRes] = await Promise.all([
    client.model.chat.default.get(),
    client.model.summary.default.get(),
    client.model.embedding.default.get(),
  ])

  const result: { chat?: Model; summary?: Model; embedding?: Model } = {}

  const chatData = chatRes.data as ApiResponse<Model> | null
  if (chatData?.success && chatData.data) {
    result.chat = chatData.data
  }

  const summaryData = summaryRes.data as ApiResponse<Model> | null
  if (summaryData?.success && summaryData.data) {
    result.summary = summaryData.data
  }

  const embeddingData = embeddingRes.data as ApiResponse<Model> | null
  if (embeddingData?.success && embeddingData.data) {
    result.embedding = embeddingData.data
  }

  return result
}

