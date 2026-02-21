package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

/*
================================================================================
QUASAR CRUD GENERATOR
================================================================================
Reads the consolidated schema (schema.logical.json) produced by the schema parser
and generates a production-ready Quasar CRUD UI scaffold:

  Per-entity:  IndexPage.vue, FormDialog.vue, DetailPage.vue, use{Entity}.ts
  Shared:      SubTableCrud.vue, PivotSelect.vue
  Global:      API client, router, validation utils, Hydra/IRI helpers,
               Zod-to-Quasar bridge, Orval config (dual vue-query + zod)

Template engine: Go text/template with [[ ]] delimiters to avoid Vue {{ }} conflict.
All templates are embedded as string constants for single-binary portability.

OUTPUT STRUCTURE:
  src-gen/
    api/client.ts
    components/SubTableCrud.vue       Reusable 1:N sub-table with inline CRUD
    components/PivotSelect.vue        Reusable M2M chip-based multi-select
    composables/use{Entity}.ts
    pages/{entity}/IndexPage.vue
    pages/{entity}/FormDialog.vue
    pages/{entity}/DetailPage.vue
    router/generated-routes.ts
    utils/validation.ts
    utils/hydra.ts
    utils/zod-to-quasar.ts
    orval.config.ts
================================================================================
*/

// ======================== Schema Types (match parser JSON output) ========================

type ConsolidatedSchema struct {
	Entities    map[string]*TableMetadata `json:"entities"`
	EntityList  []*TableMetadata          `json:"entity_list"`
	GeneratedBy string                    `json:"generated_by"`
}

type TableMetadata struct {
	StructName     string          `json:"StructName"`
	NormalizedName string          `json:"NormalizedName"`
	Source         string          `json:"Source"`
	Columns        []ColumnInfo    `json:"Columns"`
	Relations      []*RelationNode `json:"Relations"`
	Operations     []OperationInfo `json:"Operations"`
}

type ColumnInfo struct {
	Name        string            `json:"Name"`
	JSONName    string            `json:"JSONName"`
	Type        string            `json:"Type"`
	Validation  string            `json:"Validation"`
	Description string            `json:"Description"`
	Additional  string            `json:"Additional"`
	Constraints *FieldConstraints `json:"Constraints"`
	Ref         string            `json:"Ref"`
	IsArray     bool              `json:"IsArray"`
	Source      string            `json:"Source"`
}

type FieldConstraints struct {
	Required  bool     `json:"Required"`
	Nullable  bool     `json:"Nullable"`
	MinLength *int     `json:"MinLength"`
	MaxLength *int     `json:"MaxLength"`
	Minimum   *float64 `json:"Minimum"`
	Maximum   *float64 `json:"Maximum"`
	Pattern   string   `json:"Pattern"`
	Format    string   `json:"Format"`
	Enum      []string `json:"Enum"`
}

type RelationNode struct {
	FieldName    string `json:"field_name"`
	TargetStruct string `json:"target_struct"`
	IsCollection bool   `json:"is_collection"`
	TargetKey    string `json:"target_key"`
	SourceKey    string `json:"source_key"`
	Validation   string `json:"validation"`
	Description  string `json:"description"`
}

type OperationInfo struct {
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	OperationID    string   `json:"operation_id"`
	Summary        string   `json:"summary"`
	Tags           []string `json:"tags"`
	RequestSchema  string   `json:"request_schema"`
	ResponseSchema string   `json:"response_schema"`
	Source         string   `json:"source"`
}

// ======================== View Model Types ========================

type GlobalView struct {
	Entities   []EntityView
	APIBaseURL string
	OpenAPIURL string
}

type EntityView struct {
	Name            string
	NameLower       string
	NameKebab       string
	NameSnake       string
	NameHuman       string
	NamePlural      string
	NamePluralLower string
	NamePluralKebab string
	NamePluralHuman string
	APIBasePath     string

	PrimaryKey   string
	DisplayField string

	AllColumns  []ColumnView
	ListColumns []ColumnView
	FormFields  []ColumnView

	TableRelations  []RelationView
	SelectRelations []RelationView

	HasFileUpload    bool
	HasEnum          bool
	HasRelations     bool
	HasPivot         bool // M2M array-of-ID fields present
	HasNestedObjects bool // Embedded object/JSON fields present
	Operations       []OperationInfo
}

type ColumnView struct {
	Name      string
	JSONName  string
	Label     string
	GoType    string
	TSType    string
	Component string
	InputType string

	IsPrimaryKey   bool
	IsTextarea     bool
	IsFile         bool
	IsEnum         bool
	IsRelation     bool
	IsPivot        bool // M2M: array of scalar IDs
	IsNestedObject bool // Embedded object or array of objects
	IsArray        bool
	Sortable       bool
	Align          string

	RelationEntity      string
	RelationEntityLower string
	RelationEntityKebab string
	RelationAPIPath     string

	EnumOptions string
	QuasarRules string
	Required    bool
}

type RelationView struct {
	FieldName         string
	TargetEntity      string
	TargetLower       string
	TargetKebab       string
	TargetPlural      string
	TargetPluralKebab string
	TargetAPIPath     string // Full API path for fetching related items
	TargetKey         string
	SourceKey         string
	IsCollection      bool
	Description       string
}

// ======================== Template Constants — Global ========================

const tplAPIClient = `// Auto-generated API client — do not edit manually.
import axios from 'axios';

const api = axios.create({
  baseURL: '[[ .APIBaseURL ]]',
  headers: { 'Content-Type': 'application/json' },
});

// GoFrame standard response envelope
export interface GFResponse<T = any> {
  code: number;
  message: string;
  data: T;
}

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('auth_token');
  if (token && config.headers) {
    config.headers.Authorization = 'Bearer ' + token;
  }
  return config;
});

api.interceptors.response.use(
  (response) => response,
  (error) => {
    const msg = error.response?.data?.message || error.message;
    console.error('[API]', msg);
    return Promise.reject(error);
  }
);

// Unwrap GoFrame envelope and return the data payload
export function unwrap<T>(response: { data: GFResponse<T> }): T {
  const gf = response.data;
  if (gf.code !== 0) {
    throw new Error(gf.message || 'API error code: ' + gf.code);
  }
  return gf.data;
}

// Fetch relation options for QSelect async filtering
export async function fetchRelationOptions(
  entityPath: string,
  search: string,
  labelField: string,
  valueField = 'id'
): Promise<Array<{ label: string; value: any }>> {
  const res = await api.get(entityPath, { params: { search, pageSize: 20 } });
  const data = unwrap<any>(res);
  const items = Array.isArray(data) ? data : data?.list || data?.items || [];
  return items.map((item: any) => ({
    label: String(item[labelField] || item[valueField] || ''),
    value: item[valueField],
  }));
}

export default api;
`

const tplRouter = `// Auto-generated route definitions — do not edit manually.
import type { RouteRecordRaw } from 'vue-router';

const generatedRoutes: RouteRecordRaw[] = [
[[ range .Entities ]]  {
    path: '/[[ .NamePluralKebab ]]',
    name: '[[ .NamePluralKebab ]]',
    component: () => import('../pages/[[ .NameKebab ]]/IndexPage.vue'),
    meta: { title: '[[ .NamePluralHuman ]]' },
  },
  {
    path: '/[[ .NamePluralKebab ]]/:id',
    name: '[[ .NameKebab ]]-detail',
    component: () => import('../pages/[[ .NameKebab ]]/DetailPage.vue'),
    meta: { title: '[[ .NameHuman ]] Detail' },
    props: true,
  },
[[ end ]]];

export default generatedRoutes;
`

const tplValidation = `// Auto-generated validation utilities — do not edit manually.

type QRule = (val: any) => true | string;

export function required(label: string): QRule {
  return (val) => (val !== null && val !== undefined && val !== '') || label + ' is required';
}

export function minLength(label: string, min: number): QRule {
  return (val) => !val || String(val).length >= min || label + ' must be at least ' + min + ' characters';
}

export function maxLength(label: string, max: number): QRule {
  return (val) => !val || String(val).length <= max || label + ' must be at most ' + max + ' characters';
}

export function minValue(label: string, min: number): QRule {
  return (val) => val === '' || val === null || Number(val) >= min || label + ' must be >= ' + min;
}

export function maxValue(label: string, max: number): QRule {
  return (val) => val === '' || val === null || Number(val) <= max || label + ' must be <= ' + max;
}

export function pattern(label: string, regex: string): QRule {
  const re = new RegExp(regex);
  return (val) => !val || re.test(String(val)) || label + ' format is invalid';
}

export function email(label: string): QRule {
  return pattern(label, '^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$');
}
`

const tplHydra = `// Auto-generated Hydra/IRI utilities — do not edit manually.
// Handles JSON-LD resource identifiers and Hydra collection pagination.

export function extractId(iri: string | number | null | undefined): string {
  if (iri === null || iri === undefined) return '';
  if (typeof iri === 'number') return String(iri);
  const s = String(iri);
  const idx = s.lastIndexOf('/');
  return idx >= 0 ? s.substring(idx + 1) : s;
}

export function isIri(val: any): boolean {
  return typeof val === 'string' && val.startsWith('/');
}

export function buildIri(resource: string, id: string | number): string {
  return '/' + resource.replace(/^\/+/, '') + '/' + id;
}

// Hydra collection envelope
export interface HydraCollection<T = any> {
  '@context'?: string;
  '@id'?: string;
  '@type'?: string;
  'hydra:totalItems': number;
  'hydra:member': T[];
  'hydra:view'?: HydraView;
}

export interface HydraView {
  '@id': string;
  '@type': string;
  'hydra:first'?: string;
  'hydra:last'?: string;
  'hydra:next'?: string;
  'hydra:previous'?: string;
}

// Unwrap a Hydra collection or fall back to GoFrame/plain formats
export function unwrapCollection<T>(data: any): { items: T[]; total: number } {
  if (data?.['hydra:member']) {
    return {
      items: data['hydra:member'] as T[],
      total: data['hydra:totalItems'] ?? data['hydra:member'].length,
    };
  }
  const items = Array.isArray(data) ? data : data?.list || data?.items || [];
  return { items, total: data?.total ?? data?.totalCount ?? items.length };
}

export function hydraNextPage(data: any): string | null {
  return data?.['hydra:view']?.['hydra:next'] || null;
}

export function hydraPrevPage(data: any): string | null {
  return data?.['hydra:view']?.['hydra:previous'] || null;
}
`

const tplZodBridge = `// Auto-generated Zod-to-Quasar bridge — do not edit manually.
//
// Usage (after running Orval):
//   import { productCreateReqSchema } from '../api/gen/zod/products';
//   import { zodFormRules } from '../utils/zod-to-quasar';
//   const rules = zodFormRules(productCreateReqSchema);
//   // <q-input :rules="rules.name" ... />
//
import type { ZodObject, ZodTypeAny } from 'zod';

type QRule = (val: any) => true | string;

export function zodFormRules<T extends ZodObject<any>>(
  schema: T
): Record<string, QRule[]> {
  const rules: Record<string, QRule[]> = {};
  const shape = schema.shape as Record<string, ZodTypeAny>;
  for (const [field, fieldSchema] of Object.entries(shape)) {
    rules[field] = [
      (val: any) => {
        const result = fieldSchema.safeParse(val);
        if (result.success) return true;
        return result.error.issues[0]?.message || field + ' is invalid';
      },
    ];
  }
  return rules;
}

export function zodFieldRules<T extends ZodObject<any>>(
  schema: T,
  field: keyof T['shape'] & string
): QRule[] {
  const fieldSchema = schema.shape[field] as ZodTypeAny | undefined;
  if (!fieldSchema) return [];
  return [
    (val: any) => {
      const result = fieldSchema.safeParse(val);
      if (result.success) return true;
      return result.error.issues[0]?.message || field + ' is invalid';
    },
  ];
}
`

const tplOrvalConfig = `// Auto-generated Orval configuration — do not edit manually.
// Dual output: Vue Query hooks + TypeScript types, and Zod validation schemas.
// Run:  npx orval --config ./orval.config.ts
import { defineConfig } from 'orval';

export default defineConfig({
  api: {
    input: {
      target: '[[ .OpenAPIURL ]]',
    },
    output: {
      target: './src/api/gen/endpoints',
      schemas: './src/api/gen/schemas',
      client: 'vue-query',
      mode: 'tags-split',
      override: {
        mutator: {
          path: './src/api/client.ts',
          name: 'default',
        },
      },
    },
  },
  zod: {
    input: {
      target: '[[ .OpenAPIURL ]]',
    },
    output: {
      target: './src/api/gen/zod',
      client: 'zod',
      mode: 'tags-split',
    },
  },
});
`

// ======================== Template Constants — Shared Components ========================

// SubTableCrud provides embedded 1:N relation CRUD inside any detail page.
// Dynamic columns are derived from response data, so no schema lookup is needed.
const tplSubTableCrud = `<template>
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
import api, { unwrap } from '../api/client';

const props = defineProps<{
  title: string;
  apiPath: string;
  fkField: string;
  fkValue: string | number;
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
    const payload = unwrap<any>(res);
    return Array.isArray(payload) ? payload : payload?.list || payload?.items || [];
  },
  enabled: computed(() => !!props.fkValue),
});

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

const dialogOpen = ref(false);
const editItem = ref<any>(null);
const form = ref<Record<string, any>>({});
const formRef = ref<any>(null);
const saving = ref(false);

function onAdd() {
  editItem.value = null;
  form.value = { [props.fkField]: props.fkValue };
  dialogOpen.value = true;
}

function onEdit(row: any) {
  editItem.value = row;
  form.value = { ...row };
  dialogOpen.value = true;
}

const { mutateAsync: createItem } = useMutation({
  mutationFn: async (data: any) => unwrap(await api.post(props.apiPath, data)),
  onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKey.value }),
});

const { mutateAsync: updateItem } = useMutation({
  mutationFn: async (data: any) => {
    const { id, ...body } = data;
    return unwrap(await api.put(props.apiPath + '/' + id, body));
  },
  onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKey.value }),
});

const { mutateAsync: deleteItem } = useMutation({
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
  }).onOk(() => deleteItem(row.id));
}
</script>
`

// PivotSelect provides a chip-based multi-select for M2M relationships.
// Options are fetched from the target entity endpoint with type-ahead filtering.
const tplPivotSelect = `<template>
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
`

// ======================== Template Constants — Per-Entity ========================

const tplIndexPage = `<template>
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
const editedItem = ref<any>(null);

const columns = [
[[ range .ListColumns ]]  { name: '[[ .JSONName ]]', label: '[[ .Label ]]', field: '[[ .JSONName ]]', sortable: [[ .Sortable ]], align: '[[ .Align ]]' as const },
[[ end ]]  { name: 'actions', label: 'Actions', field: 'actions', align: 'center' as const },
];

function onCreate() {
  editedItem.value = null;
  dialogOpen.value = true;
}

function onEdit(row: any) {
  editedItem.value = { ...row };
  dialogOpen.value = true;
}

function onSaved() {
  dialogOpen.value = false;
  editedItem.value = null;
}

function onDelete(id: any) {
  $q.dialog({
    title: 'Confirm',
    message: 'Delete this [[ .NameLower ]]?',
    cancel: true,
    persistent: true,
  }).onOk(async () => {
    await remove(id);
  });
}
</script>
`

const tplFormDialog = `<template>
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
              :rules="[[ .QuasarRules ]]"
              class="q-pa-sm"
            />
          </q-expansion-item>
[[ else if .IsTextarea ]]          <q-input
            v-model="form.[[ .JSONName ]]"
            label="[[ .Label ]]"
            type="textarea"
            autogrow
            :rules="[[ .QuasarRules ]]"
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
            :rules="[[ .QuasarRules ]]"
          />
[[ else if .IsRelation ]]          <q-select
            v-model="form.[[ .JSONName ]]"
            label="[[ .Label ]]"
            use-input
            emit-value
            map-options
            :options="relationOpts.[[ .JSONName ]]"
            @filter="(val: string, update: any) => filterRelation(val, update, '[[ .JSONName ]]', '[[ .RelationAPIPath ]]')"
            :rules="[[ .QuasarRules ]]"
          />
[[ else if .IsPivot ]]          <PivotSelect
            v-model="form.[[ .JSONName ]]"
            label="[[ .Label ]]"
            api-path="[[ .RelationAPIPath ]]"
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
            :rules="[[ .QuasarRules ]]"
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
import { ref, reactive, computed, watch } from 'vue';
import { use[[ .Name ]] } from '../../composables/use[[ .Name ]]';
import { fetchRelationOptions } from '../../api/client';
[[ if .HasPivot ]]import PivotSelect from '../../components/PivotSelect.vue';
[[ end ]]
const props = defineProps<{
  modelValue: boolean;
  item: any | null;
}>();
const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void;
  (e: 'saved'): void;
}>();

const { create, update } = use[[ .Name ]]();
const formRef = ref<any>(null);
const saving = ref(false);

const isEdit = computed(() => props.item !== null);

const emptyForm: Record<string, any> = {
[[ range .FormFields ]]  [[ .JSONName ]]: [[ if .IsPivot ]][][[ else if .IsNestedObject ]]'{}' [[ else if eq .TSType "number" ]]0[[ else if eq .TSType "boolean" ]]false[[ else ]]''[[ end ]],
[[ end ]]};

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
`

const tplDetailPage = `<template>
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
[[ if .TableRelations ]]import SubTableCrud from '../../components/SubTableCrud.vue';
[[ end ]]
const route = useRoute();
const router = useRouter();
const $q = useQuasar();

const entityId = computed(() => route.params.id as string);
const { useItem, remove } = use[[ .Name ]]();
const { data: itemData, isLoading } = useItem(entityId);
const item = computed(() => itemData.value || null);

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
`

const tplComposable = `// Auto-generated composable for [[ .Name ]] — do not edit manually.
//
// TYPE-SAFE REWIRING (after running Orval):
//   import type { [[ .Name ]] } from '../api/gen/schemas';
//   const items = computed<[[ .Name ]][]>(() => listData.value || []);
//
import { ref, computed, type Ref } from 'vue';
import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query';
import api, { unwrap } from '../api/client';

const ENTITY_PATH = '[[ .APIBasePath ]]';
const QUERY_KEY = '[[ .NamePluralLower ]]';

export function use[[ .Name ]]() {
  const queryClient = useQueryClient();

  const pagination = ref({
    page: 1,
    rowsPerPage: 15,
    rowsNumber: 0,
    sortBy: '[[ .PrimaryKey ]]',
    descending: false,
  });

  const queryKey = computed(() => [
    QUERY_KEY,
    pagination.value.page,
    pagination.value.rowsPerPage,
    pagination.value.sortBy,
    pagination.value.descending,
  ]);

  const { data: listData, isLoading } = useQuery({
    queryKey,
    queryFn: async () => {
      const p = pagination.value;
      const res = await api.get(ENTITY_PATH, {
        params: {
          page: p.page,
          pageSize: p.rowsPerPage,
          orderBy: p.sortBy,
          orderDirection: p.descending ? 'desc' : 'asc',
        },
      });
      const payload = unwrap<any>(res);
      const list = Array.isArray(payload) ? payload : payload?.list || payload?.items || [];
      const total = payload?.total ?? payload?.totalCount ?? list.length;
      pagination.value.rowsNumber = total;
      return list;
    },
  });

  const items = computed(() => listData.value || []);

  function onRequest(props: { pagination: typeof pagination.value }) {
    pagination.value = { ...props.pagination };
  }

  function useItem(id: Ref<string | number>) {
    return useQuery({
      queryKey: computed(() => [QUERY_KEY, id.value]),
      queryFn: async () => {
        if (!id.value) return null;
        const res = await api.get(ENTITY_PATH + '/' + id.value);
        return unwrap<any>(res);
      },
      enabled: computed(() => !!id.value),
    });
  }

  const { mutateAsync: create } = useMutation({
    mutationFn: async (data: any) => {
      const res = await api.post(ENTITY_PATH, data);
      return unwrap<any>(res);
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [QUERY_KEY] }),
  });

  const { mutateAsync: update } = useMutation({
    mutationFn: async (data: any) => {
      const { [[ .PrimaryKey ]]: id, ...body } = data;
      const res = await api.put(ENTITY_PATH + '/' + id, body);
      return unwrap<any>(res);
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [QUERY_KEY] }),
  });

  const { mutateAsync: remove } = useMutation({
    mutationFn: async (id: string | number) => {
      const res = await api.delete(ENTITY_PATH + '/' + id);
      return unwrap<any>(res);
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [QUERY_KEY] }),
  });

  return { items, isLoading, pagination, onRequest, useItem, create, update, remove };
}
`

// ======================== Main ========================

func main() {
	var (
		schemaPath = flag.String("schema", "schema.logical.json", "Path to consolidated schema JSON")
		outDir     = flag.String("out", "./src-gen", "Output directory for generated files")
		apiBase    = flag.String("api-base", "/api", "API base URL prefix for composables")
		openAPIURL = flag.String("openapi-url", "http://localhost:8000/api.json", "OpenAPI spec URL for Orval")
	)
	flag.Parse()

	schema, err := loadSchema(*schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to load schema: %v\n", err)
		os.Exit(1)
	}

	var entities []EntityView
	seen := make(map[string]bool)

	sources := schema.EntityList
	if len(sources) == 0 {
		for _, m := range schema.Entities {
			sources = append(sources, m)
		}
	}

	for _, meta := range sources {
		if meta == nil || meta.NormalizedName == "" {
			continue
		}
		if seen[meta.NormalizedName] {
			continue
		}
		seen[meta.NormalizedName] = true
		if len(meta.Columns) == 0 && len(meta.Relations) == 0 {
			continue
		}
		entities = append(entities, buildEntityView(meta, *apiBase))
	}

	sort.Slice(entities, func(i, j int) bool { return entities[i].Name < entities[j].Name })

	if len(entities) == 0 {
		fmt.Println("⚠️  No entities found in schema. Nothing to generate.")
		return
	}

	global := GlobalView{
		Entities:   entities,
		APIBaseURL: *apiBase,
		OpenAPIURL: *openAPIURL,
	}

	funcMap := template.FuncMap{
		"bt": func() string { return "`" },
	}
	templates := template.New("root").Delims("[[", "]]").Funcs(funcMap)

	tplDefs := map[string]string{
		"api-client":      tplAPIClient,
		"router":          tplRouter,
		"validation":      tplValidation,
		"hydra":           tplHydra,
		"zod-bridge":      tplZodBridge,
		"orval":           tplOrvalConfig,
		"sub-table-crud":  tplSubTableCrud,
		"pivot-select":    tplPivotSelect,
		"index-page":      tplIndexPage,
		"form-dialog":     tplFormDialog,
		"detail-page":     tplDetailPage,
		"composable":      tplComposable,
	}
	for name, content := range tplDefs {
		if _, err := templates.New(name).Parse(content); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Template parse error (%s): %v\n", name, err)
			os.Exit(1)
		}
	}

	// Global scaffolding files
	globalFiles := []struct {
		tpl, path string
		data      any
	}{
		{"api-client", filepath.Join(*outDir, "api", "client.ts"), global},
		{"router", filepath.Join(*outDir, "router", "generated-routes.ts"), global},
		{"validation", filepath.Join(*outDir, "utils", "validation.ts"), nil},
		{"hydra", filepath.Join(*outDir, "utils", "hydra.ts"), nil},
		{"zod-bridge", filepath.Join(*outDir, "utils", "zod-to-quasar.ts"), nil},
		{"orval", filepath.Join(*outDir, "orval.config.ts"), global},
	}
	for _, gf := range globalFiles {
		if err := renderToFile(templates, gf.tpl, gf.path, gf.data); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		}
	}

	// Shared reusable components (no template variables)
	sharedFiles := []struct{ tpl, path string }{
		{"sub-table-crud", filepath.Join(*outDir, "components", "SubTableCrud.vue")},
		{"pivot-select", filepath.Join(*outDir, "components", "PivotSelect.vue")},
	}
	for _, sf := range sharedFiles {
		if err := renderToFile(templates, sf.tpl, sf.path, nil); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		}
	}

	// Per-entity files
	for _, ev := range entities {
		entityFiles := []struct{ tpl, path string }{
			{"index-page", filepath.Join(*outDir, "pages", ev.NameKebab, "IndexPage.vue")},
			{"form-dialog", filepath.Join(*outDir, "pages", ev.NameKebab, "FormDialog.vue")},
			{"detail-page", filepath.Join(*outDir, "pages", ev.NameKebab, "DetailPage.vue")},
			{"composable", filepath.Join(*outDir, "composables", "use"+ev.Name+".ts")},
		}
		for _, ef := range entityFiles {
			if err := renderToFile(templates, ef.tpl, ef.path, ev); err != nil {
				fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			}
		}
	}

	fmt.Printf("✅ Generated Quasar CRUD UI for %d entities in %s\n", len(entities), *outDir)
}

// ======================== Schema Loading ========================

func loadSchema(path string) (*ConsolidatedSchema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cs ConsolidatedSchema
	if err := json.Unmarshal(data, &cs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cs, nil
}

// ======================== View Model Builders ========================

func buildEntityView(meta *TableMetadata, apiBase string) EntityView {
	name := toPascal(meta.NormalizedName)
	plural := toPlural(name)

	ev := EntityView{
		Name:            name,
		NameLower:       toCamel(name),
		NameKebab:       toKebab(name),
		NameSnake:       toSnake(name),
		NameHuman:       toHuman(name),
		NamePlural:      plural,
		NamePluralLower: toCamel(plural),
		NamePluralKebab: toKebab(plural),
		NamePluralHuman: toHuman(plural),
		APIBasePath:     apiBase + "/" + toKebab(plural),
		Operations:      meta.Operations,
	}

	allCols := make([]ColumnView, 0, len(meta.Columns))
	for _, col := range meta.Columns {
		allCols = append(allCols, buildColumnView(col, apiBase))
	}
	ev.AllColumns = allCols

	ev.PrimaryKey = detectPrimaryKey(allCols)
	ev.DisplayField = detectDisplayField(allCols, ev.PrimaryKey)

	autoTimestamps := map[string]bool{
		"created_at": true, "updated_at": true, "deleted_at": true,
		"createdAt": true, "updatedAt": true, "deletedAt": true,
		"create_at": true, "update_at": true, "delete_at": true,
	}
	for _, cv := range allCols {
		if !cv.IsTextarea && !cv.IsFile {
			ev.ListColumns = append(ev.ListColumns, cv)
		}
		if !cv.IsPrimaryKey && !autoTimestamps[cv.JSONName] {
			ev.FormFields = append(ev.FormFields, cv)
		}
		if cv.IsFile {
			ev.HasFileUpload = true
		}
		if cv.IsEnum {
			ev.HasEnum = true
		}
		if cv.IsRelation || cv.IsPivot {
			ev.HasRelations = true
		}
		if cv.IsPivot {
			ev.HasPivot = true
		}
		if cv.IsNestedObject {
			ev.HasNestedObjects = true
		}
	}

	for _, rel := range meta.Relations {
		rv := buildRelationView(rel, apiBase)
		if rel.IsCollection {
			ev.TableRelations = append(ev.TableRelations, rv)
		} else {
			ev.SelectRelations = append(ev.SelectRelations, rv)
		}
	}
	if len(ev.TableRelations) > 0 || len(ev.SelectRelations) > 0 {
		ev.HasRelations = true
	}

	return ev
}

// buildColumnView resolves a single schema column into template-ready metadata,
// mapping Go types to Quasar components, detecting files/enums/relations/pivots/nested,
// and pre-computing validation rules.
func buildColumnView(col ColumnInfo, apiBase string) ColumnView {
	jsonName := col.JSONName
	if jsonName == "" {
		jsonName = toCamel(col.Name)
	}

	cv := ColumnView{
		Name:      col.Name,
		JSONName:  jsonName,
		Label:     toHuman(col.Name),
		GoType:    col.Type,
		IsArray:   col.IsArray,
		Sortable:  true,
		Align:     "left",
		Component: "q-input",
		InputType: "text",
		TSType:    "string",
	}

	if col.Constraints != nil {
		cv.Required = col.Constraints.Required
		if col.Constraints.Format != "" {
			cv.InputType = mapFormatToInputType(col.Constraints.Format)
		}
	}

	lowerJSON := strings.ToLower(jsonName)

	// Primary key detection
	if lowerJSON == "id" {
		cv.IsPrimaryKey = true
	}

	// File upload detection (by format or naming convention)
	if col.Constraints != nil && col.Constraints.Format == "binary" {
		cv.IsFile = true
	}
	if !cv.IsFile {
		fileKeywords := []string{"file", "image", "avatar", "photo", "attachment", "document", "upload", "thumbnail", "cover"}
		nameLower := strings.ToLower(col.Name)
		for _, kw := range fileKeywords {
			if strings.Contains(nameLower, kw) {
				cv.IsFile = true
				break
			}
		}
	}
	if cv.IsFile {
		cv.Component = "q-uploader"
		cv.TSType = "string"
		cv.Sortable = false
		cv.QuasarRules = buildQuasarRules(cv, col)
		return cv
	}

	// Enum detection
	if col.Constraints != nil && len(col.Constraints.Enum) > 0 {
		cv.IsEnum = true
		cv.Component = "q-select"
		cv.EnumOptions = formatEnumOptions(col.Constraints.Enum)
		cv.QuasarRules = buildQuasarRules(cv, col)
		return cv
	}

	// Pivot (M2M) detection: array of scalar IDs (e.g., role_ids: []int)
	if col.IsArray && !cv.IsPrimaryKey {
		typeLower := strings.ToLower(col.Type)
		if strings.Contains(typeLower, "int") || strings.Contains(typeLower, "uint") || strings.Contains(typeLower, "string") {
			lName := strings.ToLower(col.Name)
			if strings.HasSuffix(lName, "ids") || strings.HasSuffix(lName, "_ids") {
				cv.IsPivot = true
				cv.Component = "pivot-select"
				cv.TSType = "any[]"
				cv.Sortable = false
				rawEntity := strings.TrimSuffix(strings.TrimSuffix(lName, "_ids"), "ids")
				rawEntity = strings.TrimRight(rawEntity, "_")
				if rawEntity != "" {
					setRelationFields(&cv, normalizeEntityName(rawEntity), apiBase)
				}
				cv.QuasarRules = buildQuasarRules(cv, col)
				return cv
			}
		}
	}

	// Nested object detection: $ref without FK naming or object type without relation
	if !cv.IsPrimaryKey && !cv.IsFile && !cv.IsEnum && !cv.IsPivot {
		if col.Ref != "" {
			// Only treat as relation if field name follows FK naming convention
			hasFKSuffix := strings.HasSuffix(lowerJSON, "_id") ||
				(lowerJSON != "id" && len(lowerJSON) > 2 && strings.HasSuffix(lowerJSON, "id"))
			if !hasFKSuffix {
				// $ref without FK suffix → embedded/nested object
				cv.IsNestedObject = true
				cv.TSType = "any"
				cv.Sortable = false
				cv.QuasarRules = buildQuasarRules(cv, col)
				return cv
			}
		}
		typeLower := strings.ToLower(col.Type)
		if typeLower == "object" || (col.IsArray && col.Ref != "") {
			cv.IsNestedObject = true
			cv.TSType = "any"
			cv.Sortable = false
			cv.QuasarRules = buildQuasarRules(cv, col)
			return cv
		}
	}

	// Relation detection (by $ref with FK suffix, or naming convention)
	if !cv.IsPrimaryKey && !cv.IsFile && !cv.IsEnum && !cv.IsPivot && !cv.IsNestedObject {
		if col.Ref != "" {
			setRelationFields(&cv, normalizeEntityName(col.Ref), apiBase)
		} else if strings.HasSuffix(lowerJSON, "_id") {
			setRelationFields(&cv, normalizeEntityName(strings.TrimSuffix(lowerJSON, "_id")), apiBase)
		} else if lowerJSON != "id" && len(lowerJSON) > 2 && strings.HasSuffix(lowerJSON, "id") {
			rawEntity := strings.TrimRight(strings.TrimSuffix(lowerJSON, "id"), "_")
			if rawEntity != "" {
				setRelationFields(&cv, normalizeEntityName(rawEntity), apiBase)
			}
		}
	}

	// Type mapping (when component not yet specialized)
	if !cv.IsRelation {
		typeLower := strings.ToLower(col.Type)
		switch {
		case strings.Contains(typeLower, "int"), typeLower == "uint":
			cv.TSType = "number"
			cv.InputType = "number"
			cv.Align = "right"
		case strings.Contains(typeLower, "float"), strings.Contains(typeLower, "double"),
			strings.Contains(typeLower, "decimal"):
			cv.TSType = "number"
			cv.InputType = "number"
			cv.Align = "right"
		case typeLower == "bool", typeLower == "boolean":
			cv.TSType = "boolean"
			cv.Component = "q-toggle"
			cv.Align = "center"
		default:
			cv.TSType = "string"
			if cv.InputType == "text" && col.Constraints != nil {
				cv.InputType = mapFormatToInputType(col.Constraints.Format)
			}
		}
	}

	// Textarea detection by field name keywords (only for string q-input fields)
	if cv.Component == "q-input" && cv.TSType == "string" && !cv.IsNestedObject {
		textareaKW := []string{"description", "content", "body", "summary", "note", "comment", "bio", "text", "remark"}
		nameLower := strings.ToLower(col.Name)
		for _, kw := range textareaKW {
			if strings.Contains(nameLower, kw) {
				cv.IsTextarea = true
				cv.Sortable = false
				break
			}
		}
	}

	cv.QuasarRules = buildQuasarRules(cv, col)
	return cv
}

func setRelationFields(cv *ColumnView, target, apiBase string) {
	cv.IsRelation = true
	cv.Component = "q-select"
	cv.RelationEntity = toPascal(target)
	cv.RelationEntityLower = toCamel(target)
	cv.RelationEntityKebab = toKebab(target)
	cv.RelationAPIPath = apiBase + "/" + toKebab(toPlural(target))
}

func buildRelationView(rel *RelationNode, apiBase string) RelationView {
	target := normalizeEntityName(rel.TargetStruct)
	plural := toPlural(toPascal(target))
	return RelationView{
		FieldName:         rel.FieldName,
		TargetEntity:      toPascal(target),
		TargetLower:       toCamel(target),
		TargetKebab:       toKebab(target),
		TargetPlural:      plural,
		TargetPluralKebab: toKebab(plural),
		TargetAPIPath:     apiBase + "/" + toKebab(plural),
		TargetKey:         rel.TargetKey,
		SourceKey:         rel.SourceKey,
		IsCollection:      rel.IsCollection,
		Description:       rel.Description,
	}
}

// ======================== Detection Helpers ========================

func detectPrimaryKey(cols []ColumnView) string {
	for _, c := range cols {
		if strings.ToLower(c.JSONName) == "id" {
			return c.JSONName
		}
	}
	for _, c := range cols {
		if strings.Contains(strings.ToLower(c.JSONName), "id") && c.TSType == "number" {
			return c.JSONName
		}
	}
	if len(cols) > 0 {
		return cols[0].JSONName
	}
	return "id"
}

func detectDisplayField(cols []ColumnView, pk string) string {
	candidates := []string{"name", "title", "label", "username", "email", "slug", "display_name", "displayname"}
	for _, c := range candidates {
		for _, cv := range cols {
			if strings.ToLower(cv.JSONName) == c {
				return cv.JSONName
			}
		}
	}
	for _, cv := range cols {
		if cv.TSType == "string" && cv.JSONName != pk && !cv.IsPrimaryKey {
			return cv.JSONName
		}
	}
	return pk
}

func mapFormatToInputType(format string) string {
	switch strings.ToLower(format) {
	case "email":
		return "email"
	case "date", "date-time":
		return "date"
	case "uri", "url":
		return "url"
	case "password":
		return "password"
	case "time":
		return "time"
	default:
		return "text"
	}
}

// ======================== Validation ========================

func buildQuasarRules(cv ColumnView, col ColumnInfo) string {
	var rules []string

	if cv.Required {
		rules = append(rules, fmt.Sprintf(
			"(val: any) => (val !== null && val !== undefined && val !== '') || '%s is required'",
			escapeJSString(cv.Label)))
	}

	if col.Constraints != nil {
		c := col.Constraints
		if c.MinLength != nil {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => !val || String(val).length >= %d || '%s must be at least %d characters'",
				*c.MinLength, escapeJSString(cv.Label), *c.MinLength))
		}
		if c.MaxLength != nil {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => !val || String(val).length <= %d || '%s must be at most %d characters'",
				*c.MaxLength, escapeJSString(cv.Label), *c.MaxLength))
		}
		if c.Minimum != nil {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => val === '' || val === null || Number(val) >= %g || '%s must be >= %g'",
				*c.Minimum, escapeJSString(cv.Label), *c.Minimum))
		}
		if c.Maximum != nil {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => val === '' || val === null || Number(val) <= %g || '%s must be <= %g'",
				*c.Maximum, escapeJSString(cv.Label), *c.Maximum))
		}
		if c.Pattern != "" {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => !val || /%s/.test(String(val)) || '%s format is invalid'",
				c.Pattern, escapeJSString(cv.Label)))
		}
		if c.Format == "email" {
			rules = append(rules, fmt.Sprintf(
				"(val: any) => !val || /^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/.test(val) || '%s must be a valid email'",
				escapeJSString(cv.Label)))
		}
	}

	if len(rules) == 0 {
		return "[]"
	}
	return "[\n    " + strings.Join(rules, ",\n    ") + ",\n  ]"
}

func formatEnumOptions(enums []string) string {
	if len(enums) == 0 {
		return "[]"
	}
	parts := make([]string, len(enums))
	for i, e := range enums {
		parts[i] = fmt.Sprintf("{ label: '%s', value: '%s' }", escapeJSString(toHuman(e)), escapeJSString(e))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// ======================== Rendering ========================

func renderToFile(templates *template.Template, name, outPath string, data any) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", outPath, err)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	tpl := templates.Lookup(name)
	if tpl == nil {
		return fmt.Errorf("template %q not found", name)
	}
	if err := tpl.Execute(f, data); err != nil {
		return fmt.Errorf("execute %s: %w", name, err)
	}
	fmt.Printf("  📄 %s\n", outPath)
	return nil
}

// ======================== String Utilities ========================

func splitWords(s string) []string {
	var words []string
	var current []rune

	flush := func() {
		if len(current) > 0 {
			words = append(words, string(current))
			current = current[:0]
		}
	}

	runes := []rune(s)
	for i, r := range runes {
		if r == '_' || r == '-' || r == ' ' {
			flush()
			continue
		}
		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			if unicode.IsLower(prev) {
				flush()
			} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				flush()
			}
		}
		current = append(current, r)
	}
	flush()
	return words
}

func toPascal(s string) string {
	words := splitWords(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		r := []rune(strings.ToLower(w))
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, "")
}

func toCamel(s string) string {
	p := toPascal(s)
	if p == "" {
		return p
	}
	r := []rune(p)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

func toKebab(s string) string {
	words := splitWords(s)
	for i := range words {
		words[i] = strings.ToLower(words[i])
	}
	return strings.Join(words, "-")
}

func toSnake(s string) string {
	words := splitWords(s)
	for i := range words {
		words[i] = strings.ToLower(words[i])
	}
	return strings.Join(words, "_")
}

func toHuman(s string) string {
	words := splitWords(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		r := []rune(strings.ToLower(w))
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

func toPlural(s string) string {
	if s == "" {
		return s
	}
	lower := strings.ToLower(s)
	for _, suf := range []string{"ies", "ses", "xes", "zes", "ches", "shes"} {
		if strings.HasSuffix(lower, suf) {
			return s
		}
	}
	switch {
	case strings.HasSuffix(lower, "s"), strings.HasSuffix(lower, "x"),
		strings.HasSuffix(lower, "z"), strings.HasSuffix(lower, "ch"),
		strings.HasSuffix(lower, "sh"):
		return s + "es"
	case strings.HasSuffix(lower, "y") && len(lower) > 1:
		beforeY := lower[len(lower)-2]
		if beforeY != 'a' && beforeY != 'e' && beforeY != 'i' && beforeY != 'o' && beforeY != 'u' {
			return s[:len(s)-1] + "ies"
		}
		return s + "s"
	default:
		return s + "s"
	}
}

func normalizeEntityName(name string) string {
	cleaned := name
	suffixes := []string{
		"Req", "Request", "Res", "Response", "Input", "Output",
		"Create", "Update", "Add", "Edit", "Delete",
		"Item", "Detail", "List", "Get", "Query", "Form",
		"Dto", "DTO",
	}
	prefixes := []string{"V1", "V2", "Api"}
	for _, pre := range prefixes {
		cleaned = strings.TrimPrefix(cleaned, pre)
	}
	for _, suf := range suffixes {
		cleaned = strings.TrimSuffix(cleaned, suf)
	}
	if cleaned == "" {
		return name
	}
	return cleaned
}

func escapeJSString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}