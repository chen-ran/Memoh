<template>
  <div class="p-4">
    <section class="flex justify-between items-center">
      <h4 class="scroll-m-20 tracking-tight">
        {{ curProvider?.name }}
      </h4>
    </section>
    <Separator class="mt-4 mb-6" />

    <ProviderForm
      :provider="curProvider"
      :edit-loading="editLoading"
      :delete-loading="deleteLoading"
      @submit="changeProvider"
      @delete="deleteProvider"
    />

    <Separator class="mt-4 mb-6" />

    <ModelList
      :provider-id="curProvider?.id"
      :models="modelDataList"
      :delete-model-loading="deleteModelLoading"
      @enable="enableModel"
      @edit="handleEditModel"
      @delete="deleteModel"
    />
  </div>
</template>

<script setup lang="ts">
import { Separator } from '@memoh/ui'
import ProviderForm from './components/provider-form.vue'
import ModelList from './components/model-list.vue'
import { inject, provide, reactive, ref, toRef, watch } from 'vue'
import { type ProviderInfo, type ModelInfo } from '@memoh/shared'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import request from '@/utils/request'

// ---- Model 编辑状态（provide 给 CreateModel） ----
const openModel = reactive<{
  state: boolean
  title: 'title' | 'edit'
  curState: ModelInfo | null
}>({
  state: false,
  title: 'title',
  curState: null,
})

provide('openModel', toRef(openModel, 'state'))
provide('openModelTitle', toRef(openModel, 'title'))
provide('openModelState', toRef(openModel, 'curState'))

function handleEditModel(model: ModelInfo) {
  const copy = { ...model }
  if ('enable_as' in copy) {
    delete (copy as Record<string, unknown>).enable_as
  }
  openModel.state = true
  openModel.title = 'edit'
  openModel.curState = copy
}

// ---- 当前 Provider ----
const curProvider = inject('curProvider', ref<Partial<ProviderInfo & { id: string }>>())

// ---- API Mutations ----
const queryCache = useQueryCache()

const { mutate: deleteProvider, isLoading: deleteLoading } = useMutation({
  mutation: () => request({
    url: `/providers/${curProvider.value?.id}`,
    method: 'DELETE',
  }),
  onSettled: () => queryCache.invalidateQueries({ key: ['provider'] }),
})

const { mutate: changeProvider, isLoading: editLoading } = useMutation({
  mutation: (data: Record<string, unknown>) => request({
    url: `/providers/${curProvider.value?.id}`,
    method: 'PUT',
    data,
  }),
  onSettled: () => queryCache.invalidateQueries({ key: ['provider'] }),
})

const { mutate: deleteModel, isLoading: deleteModelLoading } = useMutation({
  mutation: (id: string) => request({
    url: `/models/model/${id}`,
    method: 'DELETE',
  }),
  onSettled: () => queryCache.invalidateQueries({ key: ['model'] }),
})

const { mutate: enableModel } = useMutation({
  mutation: (data: { as: string; model_id: string }) => request({
    url: '/models/enable',
    data,
    method: 'POST',
  }),
  onSettled: () => queryCache.invalidateQueries({ key: ['model'] }),
})

// ---- Model 列表 ----
const { data: modelDataList } = useQuery({
  key: ['model'],
  query: () => request({
    url: `/providers/${curProvider.value?.id}/models`,
  }).then((res) =>
    res.data.map((model: ModelInfo) => ({
      ...model,
      enable_as: model.enable_as ?? 'empty',
    })),
  ),
})

watch(curProvider, () => {
  queryCache.invalidateQueries({ key: ['model'] })
}, { immediate: true })
</script>
