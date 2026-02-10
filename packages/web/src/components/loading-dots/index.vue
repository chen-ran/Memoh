<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
  >
    <circle
      cx="4"
      cy="12"
      r="3"
      fill="currentColor"
    >
      <animate
        id="dot1"
        fill="freeze"
        attributeName="opacity"
        begin="0;dot3.end-0.25s"
        dur="0.75s"
        values="1;0.2"
      />
    </circle>
    <circle
      cx="12"
      cy="12"
      r="3"
      fill="currentColor"
      opacity="0.4"
    >
      <animate
        fill="freeze"
        attributeName="opacity"
        begin="dot1.begin+0.15s"
        dur="0.75s"
        values="1;0.2"
      />
    </circle>
    <circle
      cx="20"
      cy="12"
      r="3"
      fill="currentColor"
      opacity="0.3"
    >
      <animate
        id="dot3"
        fill="freeze"
        attributeName="opacity"
        begin="dot1.begin+0.3s"
        dur="0.75s"
        values="1;0.2"
      />
    </circle>
  </svg>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  size?: number
}>(), {
  size: 24,
})
</script>
