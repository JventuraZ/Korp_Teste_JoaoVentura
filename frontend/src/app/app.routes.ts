import { Routes } from '@angular/router';

/**
 * Rotas com carregamento sob demanda: cada tela vira um chunk separado, entao
 * abrir a aplicacao nao baixa o codigo da tela que o usuario ainda nao viu.
 */
export const routes: Routes = [
  { path: '', redirectTo: 'notas', pathMatch: 'full' },
  {
    path: 'produtos',
    title: 'Produtos · Korp',
    loadComponent: () => import('./produtos/produtos').then((m) => m.Produtos),
  },
  {
    path: 'produtos/:codigo',
    title: 'Produto · Korp',
    loadComponent: () => import('./produto-detalhe/produto-detalhe').then((m) => m.ProdutoDetalhe),
  },
  {
    path: 'notas',
    title: 'Notas fiscais · Korp',
    loadComponent: () => import('./notas/notas').then((m) => m.Notas),
  },
  {
    path: 'insights',
    title: 'Previsão de estoque · Korp',
    loadComponent: () => import('./insights/insights').then((m) => m.Insights),
  },
  { path: '**', redirectTo: 'notas' },
];
