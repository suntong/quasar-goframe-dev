// Auto-generated Hydra/IRI utilities — do not edit manually.
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
