// Auto-generated composable for [[ .Name ]] — do not edit manually.
//
// TYPE-SAFE REWIRING (after running Orval):
//   import type { [[ .Name ]] } from '../api/gen/schemas';
//   const items = computed<[[ .Name ]][]>(() => listData.value || []);
//
import { ref, computed, type Ref } from 'vue';
import { useQuery, useMutation, useQueryClient } from '@tanstack/vue-query';
import { api, unwrap } from '../api/client';

const ENTITY_PATH = '[[ .APIBasePath ]]';
const QUERY_KEY = '[[ .NamePluralLower ]]';

export function use[[ .Name ]]() {
  const queryClient = useQueryClient();

  const pagination = ref<{
    page: number;
    rowsPerPage: number;
    rowsNumber: number;
    sortBy: string;
    descending: boolean;
  }>({
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
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const payload = unwrap<any>(res);
      const list = Array.isArray(payload) ? payload : payload?.list || payload?.items || [];
      const total = payload?.total ?? payload?.totalCount ?? list.length;
      pagination.value.rowsNumber = total;
      return list;
    },
  });

  const items = computed(() => listData.value || []);

  function onRequest(props: { pagination: { page: number; rowsPerPage: number; rowsNumber?: number; sortBy?: string; descending?: boolean } }) {
    pagination.value = { ...props.pagination, rowsNumber: pagination.value.rowsNumber };
  }

  function useItem(id: Ref<string | number>) {
    return useQuery({
      queryKey: computed(() => [QUERY_KEY, id.value]),
      queryFn: async () => {
        if (!id.value) return null;
        const res = await api.get(ENTITY_PATH + '/' + id.value);
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        return unwrap<any>(res);
      },
      enabled: computed(() => !!id.value),
    });
  }

  const { mutateAsync: create } = useMutation({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mutationFn: async (data: any) => {
      const res = await api.post(ENTITY_PATH, data);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      return unwrap<any>(res);
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [QUERY_KEY] }),
  });

  const { mutateAsync: update } = useMutation({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    mutationFn: async (data: any) => {
      const { [[ .PrimaryKey ]]: id, ...body } = data;
      const res = await api.put(ENTITY_PATH + '/' + id, body);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      return unwrap<any>(res);
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [QUERY_KEY] }),
  });

  const { mutateAsync: remove } = useMutation({
    mutationFn: async (id: string | number) => {
      const res = await api.delete(ENTITY_PATH + '/' + id);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      return unwrap<any>(res);
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [QUERY_KEY] }),
  });

  return { items, isLoading, pagination, onRequest, useItem, create, update, remove };
}
