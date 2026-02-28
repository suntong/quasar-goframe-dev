// APIClient Auto-generated API client — do not edit manually.
import axios from 'axios';
import type { InternalAxiosRequestConfig } from 'axios';

// Named export: raw axios instance for hand-written composables and utilities
export const api = axios.create({
  baseURL: '[[ .APIBaseURL ]]',
  headers: { 'Content-Type': 'application/json' },
});

// GoFrame standard response envelope
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export interface GFResponse<T = any> {
  code: number;
  message: string;
  data: T;
}

api.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  // Check if we are in a browser environment to prevent SSR crashes
  if (typeof window !== 'undefined') {
    const token = localStorage.getItem('auth_token');
    if (token && config.headers) {
      config.headers.Authorization = 'Bearer ' + token;
    }
  }
  return config;
});

api.interceptors.response.use(
  (response) => response,
  (error) => {
    // eslint-disable-next-line @typescript-eslint/no-unsafe-member-access
    const msg = error.response?.data?.message || error.message;
    console.error('[API]', msg);
    // eslint-disable-next-line @typescript-eslint/prefer-promise-reject-errors
    return Promise.reject(error);
  }
);

// Unwrap GoFrame envelope — used by hand-written composables
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
// eslint-disable-next-line @typescript-eslint/no-explicit-any
): Promise<Array<{ label: string; value: any }>> {
  const res = await api.get(entityPath, { params: { search, pageSize: 20 } });
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const data = unwrap<any>(res);
  const items = Array.isArray(data) ? data : data?.list || data?.items || [];
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return items.map((item: any) => ({
    label: String(item[labelField] || item[valueField] || ''),
    value: item[valueField],
  }));
}

// Orval custom mutator: receives a single config object, returns Promise<T>.
// Unwraps the GoFrame envelope so Orval-generated types match the inner data shape.
export const customInstance = <T>(config: {
  url: string;
  method: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  data?: any;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  params?: any;
  headers?: Record<string, string>;
  signal?: AbortSignal;
}): Promise<T> => {
  return api(config).then((response) => unwrap<T>(response));
};
