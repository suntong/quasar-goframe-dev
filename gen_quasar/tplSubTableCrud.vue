<template>
  <q-card flat bordered class="q-mt-md">
    <q-card-section class="row items-center">
      <div class="text-subtitle1">{{ title }}</div>
      <q-space />
      <q-btn flat color="primary" icon="add" label="Add" @click="onAdd" />
    </q-card-section>

    <q-table
      :rows="items"
      :columns="tableColumns"
      :loading="isLoading"
      row-key="id"
      flat
      dense
      :pagination="{ rowsPerPage: 10 }"
    >
      <template #body-cell-_actions="props">
        <q-td :props="props">
          <q-btn flat dense icon="edit" @click="onEdit(props.row)" />
          <q-btn flat dense icon="delete" color="negative" @click="onRemove(props.row)" />
        </q-td>
      </template>
    </q-table>

    <q-dialog v-model="dialogOpen" persistent>
      <q-card style="min-width: 450px">
        <q-card-section>
          <div class="text-h6">{{ editItem ? 'Edit' : 'Add' }} {{ title }}</div>
        </q-card-section>
        <q-card-section>
          <q-form ref="formRef" class="q-gutter-sm">
            <q-input
              v-for="col in editableColumns"
              :key="col.name"
              v-model="form[col.name]"
              :label="col.label"
              :rules="rules.value[col.name] || []"
              dense
            />
          </q-form>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" v-close-popup />
          <q-btn color="primary" label="Save" :loading="saving" @click="onSave" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-card>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query';
import { useQuasar } from 'quasar';
import { api, unwrap } from '../api/client';
import { zodFormRules } from '../utils/zod-to-quasar';

const props = defineProps<{
  title: string;
  apiPath: string;
  fkField: string;
  fkValue: string | number;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  zodCreate?: any;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  zodUpdate?: any;
}>();

const $q = useQuasar();
const queryClient = useQueryClient();
const queryKey = computed(() => [props.apiPath, props.fkField, String(props.fkValue)]);

const { data: rawData, isLoading } = useQuery({
  queryKey,
  queryFn: async () => {
    if (!props.fkValue) return [];
    const res = await api.get(props.apiPath, {
      params: { [props.fkField]: props.fkValue, pageSize: 200 },
    });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const payload = unwrap<any>(res);
    return Array.isArray(payload) ? payload : payload?.list || payload?.items || [];
  },
  enabled: computed(() => !!props.fkValue),
});

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const items = computed<any[]>(() => rawData.value || []);

// Dynamic columns derived from the first data row
const tableColumns = computed(() => {
  if (!items.value.length) return [];
  const keys = Object.keys(items.value[0]).filter(
    (k) => !k.startsWith('@') && !k.startsWith('_')
  );
  const cols = keys.map((k) => ({
    name: k,
    label: k.replace(/_/g, ' ').replace(/\b\w/g, (c: string) => c.toUpperCase()),
    field: k,
    sortable: true,
    align: (typeof items.value[0][k] === 'number' ? 'right' : 'left') as 'left' | 'right' | 'center',
  }));
  cols.push({ name: '_actions', label: 'Actions', field: '_actions', sortable: false, align: 'center' as const });
  return cols;
});

// Exclude PK and FK from the inline edit form
const editableColumns = computed(() =>
  tableColumns.value.filter((c) => c.name !== 'id' && c.name !== '_actions' && c.name !== props.fkField)
);

const isEdit = computed(() => !!editItem.value?.id)
const rules = computed(() => {
  const schema = isEdit.value ? props.zodUpdate : props.zodCreate
  if (!schema || typeof schema.shape !== 'object') return {}
  // eslint-disable-next-line @typescript-eslint/no-explicit-any, @typescript-eslint/no-unnecessary-type-assertion
  return zodFormRules(schema as any)
})

const dialogOpen = ref(false);
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const editItem = ref<any>(null);
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const form = ref<Record<string, any>>({});
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const formRef = ref<any>(null);
const saving = ref(false);

function onAdd() {
  editItem.value = null;
  form.value = { [props.fkField]: props.fkValue };
  dialogOpen.value = true;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function onEdit(row: any) {
  editItem.value = row;
  form.value = { ...row };
  dialogOpen.value = true;
}

const { mutateAsync: createItem } = useMutation({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mutationFn: async (data: any) => unwrap(await api.post(props.apiPath, data)),
  onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKey.value }),
});

const { mutateAsync: updateItem } = useMutation({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mutationFn: async (data: any) => {
    const { id, ...body } = data;
    return unwrap(await api.put(props.apiPath + '/' + id, body));
  },
  onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKey.value }),
});

const { mutateAsync: deleteItem } = useMutation({
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mutationFn: async (id: any) => unwrap(await api.delete(props.apiPath + '/' + id)),
  onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKey.value }),
});

async function onSave() {
  saving.value = true;
  try {
    if (editItem.value) {
      await updateItem(form.value);
    } else {
      await createItem(form.value);
    }
    dialogOpen.value = false;
  } finally { saving.value = false; }
}

function onRemove(row: any) {
  $q.dialog({
    title: 'Confirm',
    message: 'Delete this item?',
    cancel: true,
    persistent: true,
  }).onOk(() => {
    // eslint-disable-next-line @typescript-eslint/no-floating-promises
    deleteItem(row.id);
  });
}
</script>
