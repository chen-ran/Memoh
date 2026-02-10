<script setup lang="ts">
import { RouterView } from 'vue-router'
import { Toaster } from '@memoh/ui'
import 'vue-sonner/style.css'
</script>

<template>
  <section class="[&_input]:shadow-none!">
    <RouterView />
    <Toaster position="top-center" />
  </section>
</template>
