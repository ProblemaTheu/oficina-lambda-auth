// Package token concentra o contrato de claims do JWT (F3-0.2).
//
// Os dois emissores — esta Lambda, para clientes; a aplicação, para
// funcionários — assinam com o MESMO segredo. Sem `iss`, `aud` e `tipo` a
// aplicação não conseguiria distinguir um token de cliente de um de
// funcionário, e um cliente alcançaria rotas administrativas.
package token

import "time"

const (
	// EmissorLambda identifica os tokens nascidos aqui.
	EmissorLambda = "oficina-auth-lambda"
	// Audiencia é comum aos dois emissores: quem consome é a mesma API.
	Audiencia = "oficina-api"

	// TipoCliente é o discriminador que impede escalonamento de privilégio.
	TipoCliente = "cliente"

	// Metodo é o único algoritmo aceito. Declarar explicitamente na
	// validação é o que impede o ataque de trocar o header para alg=none.
	Metodo = "HS256"

	// Validade curta porque o token é obtido só com CPF, sem senha.
	Validade = time.Hour
)
