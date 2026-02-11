<template>
  <div class="flex gap-4 items-start">
    <div class="p-2 rounded-full bg-[#F9F9F9] dark:bg-[#666]">
      <FontAwesomeIcon :icon="['fas', 'robot']" />
    </div>
    <section class="w-[90%]">
      <p class="leading-7 text-muted-foreground break-all">
        <LoadingDots v-if="message.streaming && !textContent" />
        <MarkdownRender
          v-else
          :content="textContent"
          custom-id="chat-answer"
        />
      </p>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ChatMessage } from '@/store/chat-list'
import MarkdownRender, { enableKatex, enableMermaid } from 'markstream-vue'
import LoadingDots from '@/components/loading-dots/index.vue'

enableKatex()
enableMermaid()

const props = defineProps<{
  message: ChatMessage
}>()

const textContent = computed(() => {
  return props.message.blocks
    .filter(b => b.type === 'text')
    .map(b => b.type === 'text' ? b.content : '')
    .join('')
})
</script>
