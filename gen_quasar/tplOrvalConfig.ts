// Auto-generated Orval configuration — do not edit manually.
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
