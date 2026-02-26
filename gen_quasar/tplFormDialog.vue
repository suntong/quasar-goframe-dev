<template>
  <q-dialog :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)" persistent>
    <q-card style="min-width: 500px; max-width: 700px">
      <q-card-section>
        <div class="text-h6">{{ isEdit ? 'Edit' : 'Create' }} [[ .NameHuman ]]</div>
      </q-card-section>

      <q-card-section class="scroll" style="max-height: 70vh">
        <q-form ref="formRef" @submit.prevent="onSubmit" class="q-gutter-md">
[[ range .FormFields ]][[ if .IsNestedObject ]]          <q-expansion-item label="[[ .Label ]]" icon="data_object" header-class="text-primary" class="q-mb-sm" default-opened>
            <q-input
              v-model="form.[[ .JSONName ]]"
              type="textarea"
              autogrow
              dense
              hint="JSON format"
              :rules="rules.[[ .JSONName ]]"
              class="q-pa-sm"
            />
          </q-expansion-item>
[[ else if .IsTextarea ]]          <q-input
            v-model="form.[[ .JSONName ]]"
            label="[[ .Label ]]"
            type="textarea"
            autogrow
            :rules="rules.[[ .JSONName ]]"
          />
[[ else if eq .TSType "boolean" ]]          <q-toggle
            v-model="form.[[ .JSONName ]]"
            label="[[ .Label ]]"
          />
[[ else if .IsEnum ]]          <q-select
            v-model="form.[[ .JSONName ]]"
            label="[[ .Label ]]"
            :options="[[ .EnumOptions ]]"
            emit-value
            map-options
            :rules="rules.[[ .JSONName ]]"
          />
[[ else if .IsRelation ]]          <q-select
            v-model="form.[[ .JSONName ]]"
            label="[[ .Label ]]"
            use-input
            emit-value
            map-options
            :options="relationOpts.[[ .JSONName ]]"
            @filter="(val: string, update: any) => filterRelation(val, update, '[[ .JSONName ]]', '[[ .RelationAPIPath ]]')"
            :rules="rules.[[ .JSONName ]]"
          />
[[ else if .IsPivot ]]          <PivotSelect
            v-model="form.[[ .JSONName ]]"
            label="[[ .Label ]]"
            api-path="[[ .RelationAPIPath ]]"
            :rules="rules.[[ .JSONName ]]"
          />
[[ else if .IsFile ]]          <div class="q-mb-sm">
            <q-uploader
              label="[[ .Label ]]"
              url="/api/upload"
              auto-upload
              accept="image/*,.pdf,.doc,.docx,.xls,.xlsx,.zip"
              flat
              bordered
              class="full-width"
              @uploaded="(info: any) => onFileUploaded(info, '[[ .JSONName ]]')"
            >
              <template #header="scope">
                <div class="row no-wrap items-center q-pa-sm q-gutter-xs">
                  <q-btn v-if="scope.queuedFiles.length" icon="clear_all" @click="scope.removeQueuedFiles" round dense flat>
                    <q-tooltip>Clear queue</q-tooltip>
                  </q-btn>
                  <div class="col text-subtitle2 q-pl-sm">[[ .Label ]]</div>
                  <q-btn v-if="scope.canAddFiles" icon="add_box" @click="scope.pickFiles" round dense flat>
                    <q-tooltip>Pick file</q-tooltip>
                  </q-btn>
                </div>
              </template>
            </q-uploader>
            <div v-if="form.[[ .JSONName ]]" class="q-mt-sm">
              <q-img
                v-if="isImageUrl(form.[[ .JSONName ]])"
                :src="form.[[ .JSONName ]]"
                style="max-height: 150px; max-width: 300px"
                fit="contain"
                class="rounded-borders"
              />
              <q-chip v-else removable color="secondary" text-color="white" @remove="form.[[ .JSONName ]] = ''">
                {{ form.[[ .JSONName ]] }}
              </q-chip>
            </div>
          </div>
[[ else ]]          <q-input
            v-model="form.[[ .JSONName ]]"
            label="[[ .Label ]]"[[ if ne .InputType "text" ]]
            type="[[ .InputType ]]"[[ end ]]
            :rules="rules.[[ .JSONName ]]"
          />
[[ end ]][[ end ]]        </q-form>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn flat label="Cancel" v-close-popup />
        <q-btn color="primary" label="Save" :loading="saving" @click="onSubmit" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>


<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useQuasar } from 'quasar'
import { use[[ .Name ]] } from '../../composables/use[[ .Name ]]'
import { fetchRelationOptions } from '../../api/client'
import { zodFormRules } from '../../utils/zod-to-quasar'

[[ if .ZodImportPath ]]
  [[ if or .CreateSchema .UpdateSchema ]]
    import { [[ if .CreateSchema ]][[ .CreateSchema ]][[ if ne .UpdateSchema .CreateSchema ]], [[ .UpdateSchema ]][[ end ]][[ else ]] [[ .UpdateSchema ]] [[ end ]] } from '[[ .ZodImportPath ]]'
  [[ end ]]
[[ end ]]

[[ if .HasPivot ]]
import PivotSelect from '../../components/PivotSelect.vue'
[[ end ]]

const props = defineProps<{
  modelValue: boolean;
  item: any | null;
}>();

const emit = defineEmits(['saved', 'cancel'])

const $q = useQuasar()
const saving = ref(false)

const isEdit = computed(() => props.item !== null)

const rules = computed(() => {
  const manualRules = {
    [[ range .FormFields ]]
    [[ .JSONName ]]: [[ .QuasarRules ]],
    [[ end ]]
  }

  [[ if .ZodImportPath ]]
  const schema = isEdit.value
    ? [[ if .UpdateSchema ]][[ .UpdateSchema ]][[ else ]]null[[ end ]]
    : [[ if .CreateSchema ]][[ .CreateSchema ]][[ else ]]null[[ end ]]

  if (schema && typeof (schema as any).shape === 'object') {
    return { ...manualRules, ...zodFormRules(schema as any) }
  }
  [[ end ]]

  return manualRules
})

const emptyForm: Record<string, any> = {
  [[ range .FormFields ]]
  [[ .JSONName ]]: [[ if .IsPivot ]][][[ else if .IsNestedObject ]]'{}'[[ else if eq .TSType "number" ]]0[[ else if eq .TSType "boolean" ]]false[[ else ]]' '[[ end ]],
  [[ end ]]
}

const form = reactive<Record<string, any>>({ ...emptyForm });

const relationOpts = reactive<Record<string, any[]>>({
[[ range .FormFields ]][[ if .IsRelation ]]  [[ .JSONName ]]: [],
[[ end ]][[ end ]]});

watch(() => props.item, (val) => {
  if (val) {
    const copy = { ...val };
    // Stringify embedded objects for JSON textarea editing
    for (const [k, v] of Object.entries(copy)) {
      if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
        copy[k] = JSON.stringify(v, null, 2);
      }
    }
    Object.assign(form, copy);
  } else {
    Object.assign(form, emptyForm);
  }
}, { immediate: true });

async function filterRelation(
  val: string,
  update: (fn: () => void) => void,
  fieldName: string,
  apiPath: string
) {
  const opts = await fetchRelationOptions(apiPath, val, 'name');
  update(() => { relationOpts[fieldName] = opts; });
}

function onFileUploaded(info: any, fieldName: string) {
  try {
    const res = JSON.parse(info.xhr.responseText);
    form[fieldName] = res?.data?.url || res?.url || '';
  } catch { form[fieldName] = ''; }
}

function isImageUrl(url: string): boolean {
  return /\.(jpg|jpeg|png|gif|webp|svg|bmp)(\?.*)?$/i.test(url);
}

// Parse JSON-string fields back to objects before sending to the API
function preparePayload(data: Record<string, any>): Record<string, any> {
  const out = { ...data };
  for (const [key, val] of Object.entries(out)) {
    if (typeof val === 'string') {
      const trimmed = val.trim();
      if ((trimmed.startsWith('{') && trimmed.endsWith('}')) ||
          (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
        try { out[key] = JSON.parse(trimmed); } catch { /* keep as string */ }
      }
    }
  }
  return out;
}

async function onSubmit() {
  const valid = await formRef.value?.validate();
  if (!valid) return;
  saving.value = true;
  try {
    const payload = preparePayload({ ...form });
    if (isEdit.value) {
      await update({ [[ .PrimaryKey ]]: props.item.[[ .PrimaryKey ]], ...payload });
    } else {
      await create(payload);
    }
    emit('saved');
    emit('update:modelValue', false);
  } finally {
    saving.value = false;
  }
}
</script>
