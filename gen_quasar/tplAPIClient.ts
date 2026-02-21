// Auto-generated API client — do not edit manually.
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
