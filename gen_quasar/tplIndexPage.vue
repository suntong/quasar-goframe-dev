<template>
  <q-page padding>
    <div class="row items-center q-mb-md">
      <div class="text-h5">[[ .NamePluralHuman ]]</div>
      <q-space />
      <q-btn color="primary" icon="add" label="Create" @click="onCreate" />
    </div>

    <q-table
      :rows="items"
      :columns="columns"
      :loading="isLoading"
      row-key="[[ .PrimaryKey ]]"
      v-model:pagination="pagination"
      binary-state-sort
      @request="onRequest"
    >
      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn flat dense icon="visibility" :to="'/[[ .NamePluralKebab ]]/' + props.row.[[ .PrimaryKey ]]" />
          <q-btn flat dense icon="edit" @click="onEdit(props.row)" />
          <q-btn flat dense icon="delete" color="negative" @click="onDelete(props.row.[[ .PrimaryKey ]])" />
        </q-td>
      </template>
    </q-table>

    <FormDialog v-model="dialogOpen" :item="editedItem" @saved="onSaved" />
  </q-page>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useQuasar } from 'quasar';
import { use[[ .Name ]] } from '../../composables/use[[ .Name ]]';
import FormDialog from './FormDialog.vue';

const $q = useQuasar();
const { items, isLoading, pagination, onRequest, remove } = use[[ .Name ]]();

const dialogOpen = ref(false);
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const editedItem = ref<any>(null);

const columns = [
[[ range .ListColumns ]]  { name: '[[ .JSONName ]]', label: '[[ .Label ]]', field: '[[ .JSONName ]]', sortable: [[ .Sortable ]], align: '[[ .Align ]]' as const },
[[ end ]]  { name: 'actions', label: 'Actions', field: 'actions', align: 'center' as const },
];

function onCreate() {
  editedItem.value = null;
  dialogOpen.value = true;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function onEdit(row: any) {
  editedItem.value = { ...row };
  dialogOpen.value = true;
}

function onSaved() {
  dialogOpen.value = false;
  editedItem.value = null;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function onDelete(id: any) {
  $q.dialog({
    title: 'Confirm',
    message: 'Delete this [[ .NameLower ]]?',
    cancel: true,
    persistent: true,
  }).onOk(() => {
    void remove(id);
  });
}
</script>
