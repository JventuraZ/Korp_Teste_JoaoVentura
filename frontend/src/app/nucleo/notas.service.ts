import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';

import { NotaDetalhe, PaginaNotas } from './modelos';

/** Acesso ao servico de Faturamento (C#, porta 8082 via proxy). */
@Injectable({ providedIn: 'root' })
export class NotasService {
  private readonly http = inject(HttpClient);
  private readonly base = '/api/notas';

  listar(pagina = 1, tamanho = 10): Observable<PaginaNotas> {
    const parametros = new HttpParams().set('pagina', pagina).set('tamanho', tamanho);
    return this.http.get<PaginaNotas>(this.base, { params: parametros });
  }

  obter(id: string): Observable<NotaDetalhe> {
    return this.http.get<NotaDetalhe>(`${this.base}/${id}`);
  }

  criar(): Observable<NotaDetalhe> {
    return this.http.post<NotaDetalhe>(this.base, {});
  }

  adicionarItem(id: string, codigo: string, quantidade: number): Observable<NotaDetalhe> {
    return this.http.post<NotaDetalhe>(`${this.base}/${id}/itens`, { codigo, quantidade });
  }

  removerItem(id: string, itemId: string): Observable<NotaDetalhe> {
    return this.http.delete<NotaDetalhe>(`${this.base}/${id}/itens/${itemId}`);
  }

  /**
   * Dispara a saga: baixa no Estoque, fechamento da nota e, se o fechamento
   * falhar, estorno compensatorio. Pode demorar -- o Polly repete ate 3 vezes
   * antes de desistir com 503.
   */
  imprimir(id: string): Observable<NotaDetalhe> {
    return this.http.post<NotaDetalhe>(`${this.base}/${id}/impressao`, {});
  }
}
