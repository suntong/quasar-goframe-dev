<template>
  <q-page padding>
    <div class="row items-center q-mb-md">
      <q-btn flat icon="arrow_back" label="Back" :to="'/[[ .NamePluralKebab ]]'" />
      <q-space />
      <q-btn flat icon="edit" label="Edit" @click="onEdit" />
      <q-btn flat icon="delete" label="Delete" color="negative" @click="onDelete" />
    </div>

    <q-card v-if="item" flat bordered>
      <q-card-section>
        <div class="text-h6">[[ .NameHuman ]] Detail</div>
      </q-card-section>
      <q-list separator>
[[ range .AllColumns ]][[ if .IsNestedObject ]]        <q-item>
          <q-item-section>
            <q-item-label caption>[[ .Label ]]</q-item-label>
            <pre class="text-body2 q-ma-none" style="white-space: pre-wrap">{{ formatNested(item.[[ .JSONName ]]) }}</pre>
          </q-item-section>
        </q-item>
[[ else if .IsFile ]]        <q-item>
          <q-item-section>
            <q-item-label caption>[[ .Label ]]</q-item-label>
            <div v-if="item.[[ .JSONName ]]">
              <q-img
                v-if="isImageUrl(item.[[ .JSONName ]])"
                :src="item.[[ .JSONName ]]"
                style="max-height: 200px; max-width: 400px"
                fit="contain"
                class="rounded-borders"
              />
              <a v-else :href="item.[[ .JSONName ]]" target="_blank" class="text-primary">{{ item.[[ .JSONName ]] }}</a>
            </div>
            <q-item-label v-else class="text-grey">No file</q-item-label>
          </q-item-section>
        </q-item>
[[ else ]]        <q-item>
          <q-item-section>
            <q-item-label caption>[[ .Label ]]</q-item-label>
            <q-item-label>{{ item.[[ .JSONName ]] }}</q-item-label>
          </q-item-section>
        </q-item>
[[ end ]][[ end ]]      </q-list>
    </q-card>

    <q-inner-loading :showing="isLoading" />
[[ range .TableRelations ]]
    <SubTableCrud
      title="[[ .TargetPlural ]]"
      api-path="[[ .TargetAPIPath ]]"
      fk-field="[[ .TargetKey ]]"
      :fk-value="entityId"
      :zod-create="[[ .FieldName ]]CreateSchema"
      :zod-update="[[ .FieldName ]]UpdateSchema"
    />
[[ end ]]
    <FormDialog v-model="editDialogOpen" :item="editItem" @saved="onEditSaved" />
  </q-page>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useQuasar } from 'quasar';
import { use[[ .Name ]] } from '../../composables/use[[ .Name ]]';
import FormDialog from './FormDialog.vue';
import { zodFormRules } from '../../utils/zod-to-quasar';

[[ if .TableRelations ]]
import SubTableCrud from '../../components/SubTableCrud.vue'
import { zodFormRules } from '../../utils/zod-to-quasar'
[[ end ]]

[[ range .TableRelations ]]
  [[ if .ZodImportPath ]]
    import { [[ .TargetCreateSchema ]][[ if ne .TargetUpdateSchema .TargetCreateSchema ]], [[ .TargetUpdateSchema ]][[ end ]] } from '[[ .ZodImportPath ]]'
  [[ end ]]
[[ end ]]

const route = useRoute();
const router = useRouter();
const $q = useQuasar();

const entityId = computed(() => route.params.id as string);
const { useItem, remove } = use[[ .Name ]]();
const { data: itemData, isLoading } = useItem(entityId);
const item = computed(() => itemData.value || null);

[[ range .TableRelations ]]
const [[ .FieldName ]]CreateSchema = [[ if .TargetCreateSchema ]][[ .TargetCreateSchema ]][[ else ]]null[[ end ]]
const [[ .FieldName ]]UpdateSchema = [[ if .TargetUpdateSchema ]][[ .TargetUpdateSchema ]][[ else ]][[ .FieldName ]]CreateSchema[[ end ]]
[[ end ]]

const editDialogOpen = ref(false);
const editItem = ref<any>(null);

function formatNested(val: any): string {
  if (val === null || val === undefined) return '';
  if (typeof val === 'object') return JSON.stringify(val, null, 2);
  return String(val);
}

function isImageUrl(url: string): boolean {
  return /\.(jpg|jpeg|png|gif|webp|svg|bmp)(\?.*)?$/i.test(url);
}

function onEdit() {
  editItem.value = item.value ? { ...item.value } : null;
  editDialogOpen.value = true;
}

function onEditSaved() {
  editDialogOpen.value = false;
}

function onDelete() {
  $q.dialog({
    title: 'Confirm',
    message: 'Delete this [[ .NameLower ]]?',
    cancel: true,
    persistent: true,
  }).onOk(async () => {
    await remove(entityId.value);
    router.push('/[[ .NamePluralKebab ]]');
  });
}
</script>
