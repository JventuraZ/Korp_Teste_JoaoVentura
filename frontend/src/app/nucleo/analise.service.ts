import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';

import { PainelAnomalias, PainelPrevisao } from './modelos';

/**
 * Análise preditiva sobre a trilha de movimentos, servida pelo Estoque.
 *
 * O cálculo acontece no servidor de propósito: a série de 90 dias de todos os
 * produtos não precisa trafegar até o navegador só para o cliente somar.
 */
@Injectable({ providedIn: 'root' })
export class AnaliseService {
  private readonly http = inject(HttpClient);

  /** Projeção de ruptura por produto, ordenada por urgência. */
  previsao(dias = 90): Observable<PainelPrevisao> {
    return this.http.get<PainelPrevisao>('/api/estoque/previsao', {
      params: new HttpParams().set('dias', dias),
    });
  }

  /** Baixas fora do padrão histórico do próprio produto. */
  anomalias(dias = 90): Observable<PainelAnomalias> {
    return this.http.get<PainelAnomalias>('/api/estoque/anomalias', {
      params: new HttpParams().set('dias', dias),
    });
  }
}
