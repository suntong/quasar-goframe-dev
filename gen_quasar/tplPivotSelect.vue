<template>
  <q-select
    :model-value="modelValue"
    @update:model-value="$emit('update:modelValue', $event)"
    :options="filteredOptions"
    :label="label"
    multiple
    use-chips
    use-input
    emit-value
    map-options
    :loading="loading"
    @filter="onFilter"
  >
    <template #no-option>
      <q-item>
        <q-item-section class="text-grey">No results</q-item-section>
      </q-item>
    </template>
    <template #selected-item="scope">
      <q-chip
        removable
        dense
        @remove="scope.removeAtIndex(scope.index)"
        color="primary"
        text-color="white"
      >
        {{ scope.opt.label || scope.opt }}
      </q-chip>
    </template>
  </q-select>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import api, { unwrap } from '../api/client';

const props = defineProps<{
  modelValue: any[];
  label: string;
  apiPath: string;
  labelField?: string;
  valueField?: string;
}>();

defineEmits<{
  (e: 'update:modelValue', val: any[]): void;
}>();

const lf = props.labelField || 'name';
const vf = props.valueField || 'id';

const filteredOptions = ref<Array<{ label: string; value: any }>>([]);
const loading = ref(false);

async function fetchOptions(search = '') {
  loading.value = true;
  try {
    const res = await api.get(props.apiPath, { params: { search, pageSize: 50 } });
    const data = unwrap<any>(res);
    const items = Array.isArray(data) ? data : data?.list || data?.items || [];
    filteredOptions.value = items.map((item: any) => ({
      label: String(item[lf] || item[vf]),
      value: item[vf],
    }));
  } finally { loading.value = false; }
}

function onFilter(val: string, update: (fn: () => void) => void) {
  fetchOptions(val).then(() => update(() => {}));
}

onMounted(() => fetchOptions());
</script>
