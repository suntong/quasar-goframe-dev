// Auto-generated Zod-to-Quasar bridge — do not edit manually.
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
