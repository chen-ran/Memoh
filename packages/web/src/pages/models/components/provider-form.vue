<template>
  <form @submit="editProvider">
    <div class="**:[input]:mt-3 **:[input]:mb-4">
      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          Name
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="name"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                placeholder="请输入名称"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          API 密钥
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="api_key"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                placeholder="请输入API密钥"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section>
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          URL
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="base_url"
        >
          <FormItem>
            <FormControl>
              <Input
                type="text"
                placeholder="请输入URL"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>
    </div>

    <section class="flex justify-end mt-4 gap-4">
      <ConfirmPopover
        message="确认是否删除模型平台?"
        :loading="deleteLoading"
        @confirm="$emit('delete')"
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

      <Button
        type="submit"
        :disabled="!hasChanges || !form.meta.value.valid"
      >
        <Spinner v-if="editLoading" />
        确定修改
      </Button>
    </section>
  </form>
</template>

<script setup lang="ts">
import {
  Input,
  Button,
  FormControl,
  FormField,
  FormItem,
  Spinner,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import SvgIcon from '@jamescoyle/vue-icon'
import { mdiTrashCanOutline } from '@mdi/js'
import { computed, toValue, watch } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { type ProviderInfo } from '@memoh/shared'

const props = defineProps<{
  provider: Partial<ProviderInfo & { id: string }> | undefined
  editLoading: boolean
  deleteLoading: boolean
}>()

const emit = defineEmits<{
  submit: [values: typeof form.values]
  delete: []
}>()

const providerSchema = toTypedSchema(z.object({
  name: z.string().min(1),
  base_url: z.string().min(1),
  client_type: z.string().min(1),
  api_key: z.string().min(1),
  metadata: z.object({
    additionalProp1: z.object({}),
  }),
}))

const form = useForm({
  validationSchema: providerSchema,
})

watch(() => props.provider, (newVal) => {
  if (newVal) {
    form.setValues({
      name: newVal.name,
      base_url: newVal.base_url,
      client_type: newVal.client_type,
      api_key: newVal.api_key,
    })
  }
}, { immediate: true })

const hasChanges = computed(() => {
  const raw = props.provider
  return JSON.stringify(form.values) !== JSON.stringify({
    name: raw?.name,
    base_url: raw?.base_url,
    client_type: raw?.client_type,
    api_key: raw?.api_key,
    metadata: { additionalProp1: {} },
  })
})

const editProvider = form.handleSubmit(async (value) => {
  emit('submit', value)
})
</script>
