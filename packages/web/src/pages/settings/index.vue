<template>
  <div class="max-w-187 m-auto">
    <h6 class="mt-6 mb-2 flex items-center">
      <svg-icon
        type="mdi"
        :path="mdiCog"
        class="mr-2"
      />
      显示设置
    </h6>
    <Separator />

    <div class="mt-4 space-y-4">
      <div class="flex items-center justify-between">
        <Label>语言</Label>
        <Select
          :model-value="language"
          @update:model-value="(v) => v && setLanguage(v as Locale)"
        >
          <SelectTrigger class="w-40">
            <SelectValue placeholder="选择语言" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="zh">
                中文
              </SelectItem>
              <SelectItem value="en">
                English
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>

      <Separator />

      <div class="flex items-center justify-between">
        <Label>主题</Label>
        <Select
          :model-value="theme"
          @update:model-value="(v) => v && setTheme(v as 'light' | 'dark')"
        >
          <SelectTrigger class="w-40">
            <SelectValue placeholder="选择主题" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="light">
                亮色
              </SelectItem>
              <SelectItem value="dark">
                暗色
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
    </div>

    <div class="mt-6">
      <Popover>
        <template #default="{ close }">
          <PopoverTrigger as-child>
            <Button variant="outline">
              {{ $t("login.exit") }}
            </Button>
          </PopoverTrigger>
          <PopoverContent class="w-80">
            <p class="mb-4">
              确认退出登录?
            </p>
            <div class="flex justify-end gap-3">
              <Button
                variant="outline"
                @click="close"
              >
                取消
              </Button>
              <Button @click="exit(); close()">
                确定
              </Button>
            </div>
          </PopoverContent>
        </template>
      </Popover>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Button,
  Select,
  SelectTrigger,
  SelectContent,
  SelectValue,
  SelectGroup,
  SelectItem,
  Label,
  Separator,
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@memoh/ui'
import SvgIcon from '@jamescoyle/vue-icon'
import { mdiCog } from '@mdi/js'
import { useRouter } from 'vue-router'
import { storeToRefs } from 'pinia'
import { useUserStore } from '../../store/user'
import { useSettingsStore } from '@/store/settings'
import type { Locale } from '@/i18n'

const router = useRouter()
const settingsStore = useSettingsStore()
const { language, theme } = storeToRefs(settingsStore)
const { setLanguage, setTheme } = settingsStore

const { exitLogin } = useUserStore()
const exit = () => {
  exitLogin()
  router.replace({ name: 'Login' })
}
</script>