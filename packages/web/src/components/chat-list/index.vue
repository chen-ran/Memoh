<template>
  <div
    ref="displayContainer"
    class="flex flex-col gap-4"
  >
    <template
      v-for="chatItem in messages"
      :key="chatItem.id"
    >
      <UserChat
        v-if="chatItem.role === 'user'"
        :message="chatItem"
      />
      <AssistantChat
        v-if="chatItem.role === 'assistant'"
        :message="chatItem"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import UserChat from './user-chat/index.vue'
import AssistantChat from './assistant-chat/index.vue'
import { inject, ref, watch } from 'vue'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import { useAutoScroll } from '@/composables/useAutoScroll'

const store = useChatStore()
const { messages, sendMessage } = store
const { streaming } = storeToRefs(store)

// ---- 消息发送 ----
const chatSay = inject('chatSay', ref(''))

watch(chatSay, async () => {
  if (chatSay.value) {
    const text = chatSay.value
    chatSay.value = ''
    try {
      await sendMessage(text)
    } catch {
      // ignore
    }
  }
}, { immediate: true })

// ---- 自动滚动 ----
const displayContainer = ref<HTMLElement>()
useAutoScroll(displayContainer, streaming)
</script>
