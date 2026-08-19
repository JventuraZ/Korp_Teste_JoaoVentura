import { CurrencyPipe } from '@angular/common';
import { Component, DestroyRef, Input, OnChanges, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { AccordionModule } from 'primeng/accordion';
import { AutoCompleteModule, AutoCompleteCompleteEvent } from 'primeng/autocomplete';
import { ButtonModule } from 'primeng/button';
import { CheckboxModule } from 'primeng/checkbox';
import { InputMaskModule } from 'primeng/inputmask';
import { InputNumberModule } from 'primeng/inputnumber';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';
import { MessageService } from 'primeng/api';
import { SelectModule } from 'primeng/select';
import { TableModule } from 'primeng/table';
import { TagModule } from 'primeng/tag';
import { TooltipModule } from 'primeng/tooltip';
import { catchError, finalize, forkJoin, of } from 'rxjs';

import {
  CamposPorCst, ConfiguracaoFiscal, Conflito, ContextoOperacao, ItemReferencia,
  Referencias, RegraTributaria, ResultadoSimulacao, ResultadoValidacao,
  ResumoFiscal, SituacaoFiscal,
} from '../nucleo/fiscal.modelos';
import { FiscalService } from '../nucleo/fiscal.service';
import { EditorRegra } from './editor-regra';

@Component({
  selector: 'app-aba-fiscal',
  imports: [
    FormsModule, CurrencyPipe, AccordionModule, AutoCompleteModule, ButtonModule, CheckboxModule,
    InputMaskModule, InputNumberModule, InputTextModule, MessageModule, SelectModule,
    TableModule, TagModule, TooltipModule, EditorRegra,
  ],
  templateUrl: './aba-fiscal.html',
})
export class AbaFiscal implements OnInit, OnChanges {
  private readonly api = inject(FiscalService);
  private readonly mensagens = inject(MessageService);
  private readonly destroyRef = inject(DestroyRef);

  @Input({ required: true }) codigo!: string;

  protected readonly carregando = signal(true);
  protected readonly avisoDados = signal('');

  protected readonly referencias = signal<Referencias | null>(null);
  protected readonly campos = signal<CamposPorCst | null>(null);
  protected readonly resumo = signal<ResumoFiscal | null>(null);

  protected configuracao: ConfiguracaoFiscal | null = null;
  protected readonly regras = signal<RegraTributaria[]>([]);
  protected readonly conflitos = signal<Conflito[]>([]);

  protected readonly sugestoesNcm = signal<ItemReferencia[]>([]);
  protected ncmSelecionado: ItemReferencia | string | null = null;

  // ── validação ────────────────────────────────────────────────────────
  protected readonly validacao = signal<ResultadoValidacao | null>(null);
  protected readonly validando = signal(false);

  // ── simulação ────────────────────────────────────────────────────────
  protected readonly simulacao = signal<ResultadoSimulacao | null>(null);
  protected readonly simulando = signal(false);
  protected readonly trilhaAberta = signal(false);
  protected contexto: ContextoOperacao = {
    operacao: 'VENDA', ufOrigem: 'SP', ufDestino: 'RJ', tipoCliente: 'PJ',
    consumidorFinal: false, contribuinteIcms: true, regimeEmpresa: 'NORMAL',
    finalidade: 'NORMAL',
  };
  protected valorOperacao = 1000;
  protected quantidade = 1;

  // ── editor de regra ──────────────────────────────────────────────────
  protected readonly editorAberto = signal(false);
  protected regraEmEdicao: RegraTributaria | null = null;

  ngOnInit(): void {
    this.carregarReferencias();
  }

  /** Trocar de produto sem recriar o componente exige recarregar aqui. */
  ngOnChanges(): void {
    if (this.referencias()) {
      this.carregarProduto();
    }
  }

  private carregarReferencias(): void {
    forkJoin({
      referencias: this.api.referencias().pipe(catchError(() => of(null))),
      campos: this.api.camposPorCst().pipe(catchError(() => of(null))),
    })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe(({ referencias, campos }) => {
        this.referencias.set(referencias);
        this.campos.set(campos);
        this.carregarProduto();
      });
  }

  private carregarProduto(): void {
    this.carregando.set(true);
    forkJoin({
      config: this.api.configuracao(this.codigo).pipe(catchError(() => of(null))),
      regras: this.api.regras(this.codigo).pipe(catchError(() => of(null))),
    })
      .pipe(
        finalize(() => this.carregando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe(({ config, regras }) => {
        if (config) {
          this.configuracao = config.configuracao;
          this.resumo.set(config.resumo);
          this.avisoDados.set(config.aviso);
          this.ncmSelecionado = config.configuracao.ncm;
        }
        if (regras) {
          this.regras.set(regras.itens);
          this.conflitos.set(regras.conflitos);
        }
        this.validacao.set(null);
        this.simulacao.set(null);
      });
  }

  // ── classificação ────────────────────────────────────────────────────

  protected buscarNcm(evento: AutoCompleteCompleteEvent): void {
    this.api
      .buscarNcm(evento.query)
      .pipe(catchError(() => of([])), takeUntilDestroyed(this.destroyRef))
      .subscribe((itens) => this.sugestoesNcm.set(itens));
  }

  protected aoEscolherNcm(): void {
    if (!this.configuracao) return;
    this.configuracao.ncm =
      typeof this.ncmSelecionado === 'string'
        ? this.ncmSelecionado
        : (this.ncmSelecionado?.codigo ?? '');
    this.recalcularResumo();
  }

  /**
   * O resumo é derivado, então qualquer alteração precisa pedir o recálculo ao
   * servidor — o mesmo que produz o status na carga inicial.
   */
  protected recalcularResumo(): void {
    if (!this.configuracao) return;
    this.api
      .salvar(this.codigo, this.configuracao)
      .pipe(catchError(() => of(null)), takeUntilDestroyed(this.destroyRef))
      .subscribe((resposta) => {
        if (resposta) this.resumo.set(resposta.resumo);
      });
  }

  // ── regras ───────────────────────────────────────────────────────────

  protected novaRegra(): void {
    this.regraEmEdicao = null;
    this.editorAberto.set(true);
  }

  protected editarRegra(regra: RegraTributaria): void {
    this.regraEmEdicao = regra;
    this.editorAberto.set(true);
  }

  protected duplicarRegra(regra: RegraTributaria): void {
    const copia: RegraTributaria = {
      ...structuredClone(regra),
      id: `r-${Date.now()}`,
      descricao: `${regra.descricao} (cópia)`,
      ativa: false,
    };
    this.regras.update((atual) => [...atual, copia]);
    this.mensagens.add({
      severity: 'info',
      summary: 'Regra duplicada',
      detail: 'A cópia nasce inativa para você ajustar antes de valer.',
    });
    this.reavaliar();
  }

  protected alternarAtiva(regra: RegraTributaria): void {
    this.regras.update((atual) =>
      atual.map((r) => (r.id === regra.id ? { ...r, ativa: !r.ativa } : r)),
    );
    this.reavaliar();
  }

  protected aoSalvarRegra(regra: RegraTributaria): void {
    this.regras.update((atual) => {
      const existe = atual.some((r) => r.id === regra.id);
      return existe ? atual.map((r) => (r.id === regra.id ? regra : r)) : [...atual, regra];
    });
    this.editorAberto.set(false);
    this.reavaliar();
  }

  /**
   * Reavalia conflitos e resumo depois de mexer nas regras.
   *
   * A detecção roda no servidor mesmo no protótipo: é o mesmo algoritmo que o
   * motor usa para escolher a regra, e duplicá-lo no frontend garantiria que
   * um dia os dois discordassem.
   */
  private reavaliar(): void {
    if (!this.configuracao) return;
    this.api
      .validar(this.codigo, this.configuracao, this.regras())
      .pipe(catchError(() => of(null)), takeUntilDestroyed(this.destroyRef))
      .subscribe((resultado) => {
        if (!resultado) return;
        this.validacao.set(resultado);
        this.conflitos.set(
          resultado.problemas
            .filter((p) => p.area === 'Regras por operação' && p.mensagem.includes('valem para as mesmas'))
            .map((p) => ({
              severidade: p.gravidade === 'ERRO' ? 'AMBIGUA' : 'SOBREPOSTA',
              regraA: '', regraB: '', explicacao: p.mensagem,
            })),
        );
      });
    this.recalcularResumo();
  }

  protected regraEmConflito(regra: RegraTributaria): boolean {
    return this.conflitos().some((c) => c.explicacao.includes(`"${regra.descricao}"`));
  }

  // ── validação e simulação ────────────────────────────────────────────

  protected validar(): void {
    if (!this.configuracao) return;
    this.validando.set(true);
    this.api
      .validar(this.codigo, this.configuracao, this.regras())
      .pipe(
        catchError(() => of(null)),
        finalize(() => this.validando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((resultado) => {
        if (!resultado) return;
        this.validacao.set(resultado);
        this.mensagens.add({
          severity: resultado.valida ? 'success' : 'warn',
          summary: resultado.valida ? 'Configuração válida' : 'Pendências encontradas',
          detail: resultado.valida
            ? 'Nenhum impedimento estrutural para emissão.'
            : `${resultado.problemas.length} ponto(s) para revisar.`,
        });
      });
  }

  protected simular(): void {
    this.simulando.set(true);
    this.api
      .simular(this.codigo, this.contexto, this.valorOperacao, this.quantidade, this.regras())
      .pipe(
        catchError(() => of(null)),
        finalize(() => this.simulando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((resultado) => {
        this.simulacao.set(resultado);
        if (resultado && !resultado.encontrou) {
          this.mensagens.add({
            severity: 'warn',
            summary: 'Nenhuma regra corresponde',
            detail: 'Não há regra ativa para esta combinação de operação.',
          });
        }
      });
  }

  // ── apresentação ─────────────────────────────────────────────────────

  /** Campos de ICMS aplicáveis ao CST/CSOSN escolhido — vindos do servidor. */
  protected camposDoIcms(cst: string, csosn: string): string[] {
    const mapa = this.campos();
    if (!mapa) return [];
    if (csosn) return mapa.csosn[csosn] ?? [];
    return mapa.icms[cst] ?? [];
  }

  protected rotulo(campo: string): string {
    return this.campos()?.rotulos[campo] ?? campo;
  }

  protected severidade(situacao: SituacaoFiscal | undefined): 'success' | 'warn' | 'danger' | 'secondary' {
    switch (situacao) {
      case 'CONFIGURADO': return 'success';
      case 'INCOMPLETO': return 'warn';
      case 'ERRO': return 'danger';
      default: return 'secondary';
    }
  }

  protected rotuloSituacao(situacao: SituacaoFiscal | undefined): string {
    switch (situacao) {
      case 'CONFIGURADO': return 'Configurado';
      case 'INCOMPLETO': return 'Incompleto';
      case 'ERRO': return 'Erro fiscal';
      default: return 'Não aplicável';
    }
  }

  protected areasDoResumo(): { nome: string; situacao: SituacaoFiscal }[] {
    const resumo = this.resumo();
    if (!resumo) return [];
    return Object.entries(resumo.areas).map(([nome, situacao]) => ({ nome, situacao }));
  }

  protected descreverCondicoes(regra: RegraTributaria): string {
    const partes: string[] = [];
    const c = regra.condicoes;
    if (c.operacao) partes.push(c.operacao);
    if (c.ufOrigem) partes.push(`de ${c.ufOrigem}`);
    if (c.ufDestino) partes.push(`para ${c.ufDestino}`);
    if (c.tipoCliente) partes.push(c.tipoCliente);
    if (c.consumidorFinal !== null) partes.push(c.consumidorFinal ? 'consumidor final' : 'não consumidor final');
    if (c.contribuinteIcms !== null) partes.push(c.contribuinteIcms ? 'contribuinte' : 'não contribuinte');
    return partes.length ? partes.join(' · ') : 'qualquer operação';
  }
}
