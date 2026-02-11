<template>
  <div class="flex">
    <p
      class="leading-7 not-first:mt-6 max-w-[90%] ml-auto text-muted-foreground bg-[#F9F9F9] p-4 rounded-xl rounded-tr-none break-all dark:bg-[#1C1917]
"
    >
      {{ textContent }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ChatMessage } from '@/store/chat-list'

const props = defineProps<{
  message: ChatMessage
}>()

const textContent = computed(() => {
  const block = props.message.blocks.find(b => b.type === 'text')
  return block?.type === 'text' ? block.content : ''
})
</script>
