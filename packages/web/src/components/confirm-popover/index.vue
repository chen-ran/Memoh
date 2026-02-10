<template>
  <Popover>
    <template #default="{ close }">
      <PopoverTrigger as-child>
        <slot name="trigger" />
      </PopoverTrigger>
      <PopoverContent class="w-80">
        <p class="mb-4">
          {{ message }}
        </p>
        <div class="flex justify-end gap-3">
          <Button
            variant="outline"
            @click="close"
          >
            {{ cancelText }}
          </Button>
          <Button
            :disabled="loading"
            @click="$emit('confirm'); close()"
          >
            <Spinner v-if="loading" />
            {{ confirmText }}
          </Button>
        </div>
      </PopoverContent>
    </template>
  </Popover>
</template>

<script setup lang="ts">
import {
  Button,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Spinner,
} from '@memoh/ui'

withDefaults(defineProps<{
  message: string
  confirmText?: string
  cancelText?: string
  loading?: boolean
}>(), {
  confirmText: '确定',
  cancelText: '取消',
  loading: false,
})

defineEmits<{
  confirm: []
}>()
</script>
