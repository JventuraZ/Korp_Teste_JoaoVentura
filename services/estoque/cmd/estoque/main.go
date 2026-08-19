// Comando estoque: sobe o servico de Estoque.
//
// Ao iniciar, aplica as migrations embutidas no binario e so entao abre a
// porta. O container fica saudavel apenas depois de o schema estar correto --
// e por isso que o compose consegue subir tudo do zero num comando so.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/joaoventura/korp-estoque/internal/repositorio"
	"github.com/joaoventura/korp-estoque/internal/transporte"
	migracoes "github.com/joaoventura/korp-estoque/migrations"
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
	porta := valorOuPadrao("ESTOQUE_PORT", "8081")
	urlBanco := os.Getenv("ESTOQUE_DATABASE_URL")
	if urlBanco == "" {
		return errors.New("ESTOQUE_DATABASE_URL nao definida")
	}

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()

	if err := aplicarMigrations(urlBanco); err != nil {
		return fmt.Errorf("aplicar migrations: %w", err)
	}

	pool, err := pgxpool.New(ctx, urlBanco)
	if err != nil {
		return fmt.Errorf("abrir pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("conectar ao banco: %w", err)
	}

	servidor := &http.Server{
		Addr:              ":" + porta,
		Handler:           transporte.NovoRoteador(repositorio.Novo(pool)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	erros := make(chan error, 1)
	go func() {
		slog.Info("estoque ouvindo", "porta", porta)
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

// aplicarMigrations roda o goose sobre os .sql embutidos, usando uma conexao
// database/sql descartavel -- o goose nao fala pgx nativamente.
func aplicarMigrations(urlBanco string) error {
	config, err := pgxpool.ParseConfig(urlBanco)
	if err != nil {
		return fmt.Errorf("interpretar url do banco: %w", err)
	}

	banco := stdlib.OpenDB(*config.ConnConfig)
	defer banco.Close()

	if err := aguardarBanco(banco); err != nil {
		return err
	}

	goose.SetBaseFS(migracoes.Arquivos)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(banco, ".")
}

// aguardarBanco tolera o Postgres ainda subindo. O compose ja usa depends_on
// com healthcheck, mas rodando nativo no host essa corrida acontece.
func aguardarBanco(banco *sql.DB) error {
	var err error
	for tentativa := 0; tentativa < 10; tentativa++ {
		if err = banco.Ping(); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("banco indisponivel apos 10 tentativas: %w", err)
}

func executarHealthcheck() int {
	porta := valorOuPadrao("ESTOQUE_PORT", "8081")
	cliente := &http.Client{Timeout: 3 * time.Second}

	resposta, err := cliente.Get("http://localhost:" + porta + "/health")
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
