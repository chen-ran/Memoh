<template>
  <section>
    <section class="flex justify-between items-center mb-4">
      <h4 class="scroll-m-20 font-semibold tracking-tight">
        模型
      </h4>
      <CreateModel
        v-if="providerId"
        :id="providerId"
      />
    </section>

    <section
      v-if="models?.length > 0"
      class="flex flex-col gap-4"
    >
      <ModelItem
        v-for="model in models"
        :key="model.model_id"
        :model="model"
        :delete-loading="deleteModelLoading"
        @enable="(payload) => $emit('enable', payload)"
        @edit="(model) => $emit('edit', model)"
        @delete="(name) => $emit('delete', name)"
      />
    </section>

    <Empty
      v-else
      class="h-full flex justify-center items-center"
    >
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <svg-icon
            type="mdi"
            :path="mdiListBoxOutline"
          />
        </EmptyMedia>
      </EmptyHeader>
      <EmptyTitle>还没有添加模型</EmptyTitle>
      <EmptyDescription>请为当前Provider添加模型</EmptyDescription>
      <EmptyContent />
    </Empty>
  </section>
</template>

<script setup lang="ts">
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@memoh/ui'
import CreateModel from '@/components/create-model/index.vue'
import ModelItem from './model-item.vue'
import SvgIcon from '@jamescoyle/vue-icon'
import { mdiListBoxOutline } from '@mdi/js'
import { type ModelInfo } from '@memoh/shared'

defineProps<{
  providerId: string | undefined
  models: (ModelInfo & { enable_as: string })[] | undefined
  deleteModelLoading: boolean
}>()

defineEmits<{
  enable: [payload: { as: string; model_id: string }]
  edit: [model: ModelInfo]
  delete: [name: string]
}>()
</script>
