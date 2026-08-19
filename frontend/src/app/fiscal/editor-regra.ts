import { Component, EventEmitter, Input, OnChanges, Output, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ButtonModule } from 'primeng/button';
import { CheckboxModule } from 'primeng/checkbox';
import { DialogModule } from 'primeng/dialog';
import { InputNumberModule } from 'primeng/inputnumber';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';
import { SelectModule } from 'primeng/select';

import {
  CamposPorCst, Referencias, RegraTributaria, condicoesVazias, resultadoVazio,
} from '../nucleo/fiscal.modelos';

/**
 * Editor de uma regra tributária.
 *
 * As condições usam `null` como curinga — "qualquer UF", "qualquer cliente".
 * É isso que permite escrever a regra geral da empresa sem enumerar 27 estados,
 * e é também o que define a especificidade que decide qual regra vence.
 */
@Component({
  selector: 'app-editor-regra',
  imports: [
    FormsModule, ButtonModule, CheckboxModule, DialogModule, InputNumberModule,
    InputTextModule, MessageModule, SelectModule,
  ],
  templateUrl: './editor-regra.html',
})
export class EditorRegra implements OnChanges {
  @Input() aberto = false;
  @Input() regra: RegraTributaria | null = null;
  @Input() referencias: Referencias | null = null;
  @Input() campos: CamposPorCst | null = null;

  @Output() readonly salvar = new EventEmitter<RegraTributaria>();
  @Output() readonly fechar = new EventEmitter<void>();

  protected readonly emEdicao = signal(false);
  protected rascunho: RegraTributaria = this.novaRegra();

  /** Opções de "sim/não/qualquer" — o terceiro valor é o curinga. */
  protected readonly opcoesTernarias = [
    { rotulo: 'Qualquer', valor: null },
    { rotulo: 'Sim', valor: true },
    { rotulo: 'Não', valor: false },
  ];

  ngOnChanges(): void {
    if (!this.aberto) return;

    this.emEdicao.set(this.regra !== null);
    this.rascunho = this.regra ? structuredClone(this.regra) : this.novaRegra();
  }

  private novaRegra(): RegraTributaria {
    return {
      id: `r-${Date.now()}`,
      descricao: '',
      prioridade: 10,
      ativa: true,
      condicoes: condicoesVazias(),
      resultado: resultadoVazio(),
    };
  }

  /**
   * Quantas condições a regra fixa. Mostrado ao usuário porque é o critério
   * que decide qual regra vence — deixar isso implícito geraria a dúvida
   * "por que a outra regra foi usada?".
   */
  protected especificidade(): number {
    const c = this.rascunho.condicoes;
    return [
      c.operacao, c.ufOrigem, c.ufDestino, c.tipoCliente,
      c.consumidorFinal, c.contribuinteIcms, c.regimeEmpresa, c.finalidade,
    ].filter((v) => v !== null && v !== undefined && v !== '').length;
  }

  /** Campos de ICMS aplicáveis ao CST/CSOSN — o mapa vem do servidor. */
  protected camposAplicaveis(): string[] {
    if (!this.campos) return [];
    const { cstIcms, csosn } = this.rascunho.resultado;
    if (csosn) return this.campos.csosn[csosn] ?? [];
    return this.campos.icms[cstIcms] ?? [];
  }

  protected rotulo(campo: string): string {
    return this.campos?.rotulos[campo] ?? campo;
  }

  protected aplicavel(campo: string): boolean {
    return this.camposAplicaveis().includes(campo);
  }

  protected valorDe(campo: string): number | null {
    return (this.rascunho.resultado as unknown as Record<string, number | null>)[campo] ?? null;
  }

  protected definir(campo: string, valor: number | null): void {
    (this.rascunho.resultado as unknown as Record<string, number | null>)[campo] = valor;
  }

  protected podeSalvar(): boolean {
    return this.rascunho.descricao.trim().length > 0;
  }

  protected confirmar(): void {
    this.salvar.emit(this.rascunho);
  }
}
