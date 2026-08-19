// Package migracoes embute os arquivos .sql no binario.
//
// As migrations viajam dentro do executavel: o container sobe e se auto-migra,
// sem passo manual, sem sidecar e sem depender de o .sql estar na imagem final.
// E o que permite ao compose subir do zero com um unico comando.
package migracoes

import "embed"

//go:embed *.sql
var Arquivos embed.FS
