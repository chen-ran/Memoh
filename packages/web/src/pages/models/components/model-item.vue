<template>
  <Item variant="outline">
    <ItemContent>
      <ItemTitle>{{ model.name }}</ItemTitle>
      <ItemDescription class="gap-2 flex flex-wrap items-center mt-3">
        <Badge variant="outline">
          {{ model.type }}
        </Badge>
      </ItemDescription>
    </ItemContent>
    <ItemActions>
      <Select
        :default-value="model.enable_as"
        @update:model-value="(value) => $emit('enable', {
          as: value === 'empty' ? '' : (value as string),
          model_id: model.model_id,
        })"
      >
        <SelectTrigger class="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value="empty">
              No Enable
            </SelectItem>
            <SelectItem value="chat">
              Chat
            </SelectItem>
            <SelectItem value="embedding">
              Embedding
            </SelectItem>
            <SelectItem value="memery">
              Memery
            </SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>

      <Button
        variant="outline"
        class="cursor-pointer"
        @click="$emit('edit', model)"
      >
        <svg-icon
          type="mdi"
          :path="mdiCog"
        />
      </Button>

      <ConfirmPopover
        message="确认是否删除模型?"
        :loading="deleteLoading"
        @confirm="$emit('delete', model.name)"
      >
        <template #trigger>
          <Button variant="outline">
            <svg-icon
              type="mdi"
              :path="mdiTrashCanOutline"
            />
          </Button>
        </template>
      </ConfirmPopover>
    </ItemActions>
  </Item>
</template>

<script setup lang="ts">
import {
  Item,
  ItemContent,
  ItemDescription,
  ItemActions,
  ItemTitle,
  Badge,
  Button,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import SvgIcon from '@jamescoyle/vue-icon'
import { mdiCog, mdiTrashCanOutline } from '@mdi/js'
import { type ModelInfo } from '@memoh/shared'

defineProps<{
  model: ModelInfo & { enable_as: string }
  deleteLoading: boolean
}>()

defineEmits<{
  enable: [payload: { as: string; model_id: string }]
  edit: [model: ModelInfo]
  delete: [name: string]
}>()
</script>
