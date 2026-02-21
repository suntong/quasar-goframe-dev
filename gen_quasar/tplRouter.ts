// Auto-generated route definitions — do not edit manually.
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
