// Comando fiscal: servico de configuracao tributaria de produtos.
//
// Terceiro microsservico da stack. Nao tem banco: nesta fase serve tabelas de
// referencia embutidas e avalia regras enviadas pelo cliente. A fronteira,
// porem, ja e a definitiva -- quando houver persistencia, muda o repositorio,
// nao o contrato.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joaoventura/korp-fiscal/internal/dados"
	"github.com/joaoventura/korp-fiscal/internal/transporte"
)

func main() {
	verificarSaude := flag.Bool("healthcheck", false, "consulta /health e sai com 0 ou 1")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if *verificarSaude {
		os.Exit(executarHealthcheck())
	}
	if err := executar(); err != nil {
		slog.Error("servico encerrado com erro", "erro", err)
		os.Exit(1)
	}
}

func executar() error {
	porta := valorOuPadrao("FISCAL_PORT", "8083")

	// Falhar na subida, e nao no primeiro request: dado de referencia
	// corrompido e problema de implantacao, nao de uso.
	catalogo, err := dados.Carregar()
	if err != nil {
		return err
	}
	slog.Info("catalogo fiscal carregado",
		"cstIcms", len(catalogo.Referencias.CSTIcms),
		"csosn", len(catalogo.Referencias.CSOSN),
		"produtosExemplo", len(catalogo.Exemplos.Configuracoes))

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()

	servidor := &http.Server{
		Addr:              ":" + porta,
		Handler:           transporte.NovoRoteador(catalogo),
		ReadHeaderTimeout: 5 * time.Second,
	}

	erros := make(chan error, 1)
	go func() {
		slog.Info("fiscal ouvindo", "porta", porta)
		if err := servidor.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			erros <- err
		}
	}()

	select {
	case err := <-erros:
		return err
	case <-ctx.Done():
		slog.Info("encerrando")
		desligamento, cancelarDesligamento := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelarDesligamento()
		return servidor.Shutdown(desligamento)
	}
}

func executarHealthcheck() int {
	cliente := &http.Client{Timeout: 3 * time.Second}
	resposta, err := cliente.Get("http://127.0.0.1:" + valorOuPadrao("FISCAL_PORT", "8083") + "/health")
	if err != nil {
		return 1
	}
	defer resposta.Body.Close()
	if resposta.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func valorOuPadrao(chave, padrao string) string {
	if valor := os.Getenv(chave); valor != "" {
		return valor
	}
	return padrao
}
