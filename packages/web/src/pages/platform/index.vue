<template>
  <section>
    <AddPlatform />

    <menu class="grid grid-cols-4 gap-4 [&_li>*]:h-full">
      <PlatformCard
        v-for="item in platformList"
        :key="item.name"
        :platform="item"
        @edit="() => { open = true }"
      />
    </menu>
  </section>
</template>

<script setup lang="ts">
import { useQuery } from '@pinia/colada'
import request from '@/utils/request'
import { computed, provide, ref } from 'vue'
import AddPlatform from '@/components/add-platform/index.vue'
import PlatformCard from './components/platform-card.vue'

const open = ref(false)
provide('open', open)

const { data: platformData } = useQuery({
  key: ['platform'],
  query: () => request({ url: '/platform/' }),
})

const platformList = computed(() => platformData.value?.data ?? [])
</script>
