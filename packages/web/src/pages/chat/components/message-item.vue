<template>
  <div
    class="flex gap-3 items-start"
    :class="message.role === 'user' ? 'justify-end' : ''"
  >
    <!-- Assistant avatar -->
    <div
      v-if="message.role === 'assistant'"
      class="shrink-0 size-8 rounded-full bg-primary/10 flex items-center justify-center"
    >
      <FontAwesomeIcon
        :icon="['fas', 'robot']"
        class="size-4 text-primary"
      />
    </div>

    <!-- Content -->
    <div
      class="min-w-0"
      :class="message.role === 'user' ? 'max-w-[80%]' : 'flex-1 max-w-full'"
    >
      <!-- User message -->
      <div
        v-if="message.role === 'user'"
        class="rounded-2xl rounded-tr-sm bg-primary text-primary-foreground px-4 py-2.5 text-sm whitespace-pre-wrap"
      >
        {{ (message.blocks[0] as TextBlock)?.content }}
      </div>

      <!-- Assistant message blocks -->
      <div
        v-else
        class="space-y-3"
      >
        <template
          v-for="(block, i) in message.blocks"
          :key="i"
        >
          <!-- Thinking block -->
          <ThinkingBlock
            v-if="block.type === 'thinking'"
            :block="(block as ThinkingBlockType)"
            :streaming="message.streaming && !block.done"
          />

          <!-- Tool call block -->
          <ToolCallBlock
            v-else-if="block.type === 'tool_call'"
            :block="(block as ToolCallBlockType)"
          />

          <!-- Text block -->
          <div
            v-else-if="block.type === 'text' && block.content"
            class="prose prose-sm dark:prose-invert max-w-none *:first:mt-0"
          >
            <MarkdownRender
              :content="block.content"
              custom-id="chat-msg"
            />
          </div>
        </template>

        <!-- Streaming indicator -->
        <div
          v-if="message.streaming && message.blocks.length === 0"
          class="flex items-center gap-2 text-sm text-muted-foreground h-8"
        >
          <FontAwesomeIcon
            :icon="['fas', 'spinner']"
            class="size-3.5 animate-spin"
          />
          {{ $t('chat.thinking') }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import MarkdownRender, { enableKatex, enableMermaid } from 'markstream-vue'

enableKatex()
enableMermaid()
import ThinkingBlock from './thinking-block.vue'
import ToolCallBlock from './tool-call-block.vue'
import type {
  ChatMessage,
  TextBlock,
  ThinkingBlock as ThinkingBlockType,
  ToolCallBlock as ToolCallBlockType,
} from '@/store/chat-list'

defineProps<{
  message: ChatMessage
}>()
</script>
