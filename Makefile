# ── Variáveis ──────────────────────────────────────────────

DOCKER_COMPOSE = docker compose
SERVICE_NAME = ledger
DB_URL  = postgres://admin:admin@localhost:5432/isura_ledger_main?sslmode=disable
SCHEMA_DIR = db

# ── Variáveis de Teste de Carga (Vegeta) ─────────────────────
PATH_VEGETA ?= ./vegeta/account/create_account.go
PATH_SEED   ?= ./vegeta/transaction/seed_accounts.go
URL         ?= http://localhost:8080/accounts
DURATION    ?= 60s
RATE        ?= 1000
CONNECTIONS ?= 5000
WORKERS     ?= 500

help:
	@echo "======================================================================"
	@echo " 🚀 ISURA LEDGER MS - COMANDOS DISPONÍVEIS"
	@echo "======================================================================"
	@echo ""
	@echo " [ Aplicação & Execução ]"
	@echo "   make run-app          - Executa a aplicação localmente"
	@echo "   make run-race         - Executa a aplicação com detecção de race conditions"
	@echo "   make run-app-logs     - Executa a aplicação exportando logs para arquivo"
	@echo "   make air              - Executa com hot-reload (via Air)"
	@echo ""
	@echo " [ Testes Unitários & Cobertura ]"
	@echo "   make unit             - Roda os testes unitários básicos"
	@echo "   make unit-verbose     - Roda os testes unitários via Ginkgo com race detector"
	@echo "   make unit-cover       - Roda testes unitários medindo cobertura"
	@echo "   make unit-report      - Gera relatório HTML e de funções da cobertura"
	@echo ""
	@echo " [ Testes de Carga (Vegeta) ]"
	@echo "   make test-load        - Roda teste de carga (Variáveis: PATH_VEGETA, URL, RATE, CONNECTIONS, WORKERS, DURATION) "
	@echo ""
	@echo " [ Docker & Infraestrutura ]"
	@echo "   make build            - Faz build da imagem local sem cache"
	@echo "   make up               - Sobe o ambiente completo (DB + App)"
	@echo "   make down             - Para e remove os containers"
	@echo "   make logs             - Exibe os logs do container em tempo real"
	@echo "   make bash             - Entra no terminal do container da aplicação"
	@echo "   make restart          - Para, faz o build e sobe o ambiente novamente"
	@echo ""
	@echo " [ Protobuf & Migrations ]"
	@echo "   make proto-lint       - Valida a sintaxe dos arquivos .proto com Buf"
	@echo "   make proto-gen        - Gera o código Go a partir dos protos"
	@echo "   make migrate          - Aplica as migrations do banco via Atlas"
	@echo "======================================================================"

run-app:
	@echo "Running app"
	@go run cmd/server/main.go
run-race:
	@echo "Running race active"
	go run -race cmd/server/main.go


run-app-logs:
	@echo "Running app export archive logs"
	@go run cmd/main.go > ~/tmp/app/customers-ms.log 2>&1

air:
	@echo "Running with reload"
	@air -c .air.toml

unit:
	@go test ./tests/unit/... --tags=unit -v

unit-verbose:
	ginkgo -r --race --tags=unit --randomize-all --randomize-suites --fail-on-pending

unit-cover:
	@go test ./tests/unit/... -coverpkg ./internal/... --tags=unit

unit-report:
	mkdir -p "coverage" \
	&& go test ./tests/unit/... -coverprofile=coverage/cover.out -coverpkg ./internal/... --tags=unit \
	&& go tool cover -html=coverage/cover.out -o coverage/cover.html \
	&& go tool cover -func=coverage/cover.out -o coverage/cover.functions.html

test-load:
	@if echo "$(PATH_VEGETA)" | grep -q "transaction"; then \
		if [ ! -f accounts_pool.json ]; then \
			echo "📦 'accounts_pool.json' não encontrado. A gerar o pool de 1000 contas automaticamente..."; \
			go run $(PATH_SEED) -count=1000; \
		else \
			echo "📂 Pool de contas detetado (accounts_pool.json). A saltar a fase de seed."; \
		fi; \
	fi
	@echo "🚀 Iniciando teste de carga estável ($(RATE) req/s) usando $(PATH_VEGETA)..."
	go run $(PATH_VEGETA) -rate=$(RATE) -connections=$(CONNECTIONS) -workers=$(WORKERS) -duration=$(DURATION) -url=$(URL)


# 1. Build da imagem local usando o Dockerfile.local
build:
	$(DOCKER_COMPOSE) build --no-cache $(SERVICE_NAME)

# 2. Sobe o ambiente completo (DB + App com Air)
up:
	$(DOCKER_COMPOSE) up -d

# 3. Para tudo e remove containers
down:
	$(DOCKER_COMPOSE) down

# 4. Ver logs do Air/App em tempo real
logs:
	$(DOCKER_COMPOSE) logs -f $(SERVICE_NAME)

# 5. Atalho para entrar no container (útil para rodar migrations manuais)
bash:
	docker exec -it $$(docker ps -q -f name=$(SERVICE_NAME)) bash

# 6. Build + Up combinado
restart: down build up logs

# Geração de stubs Protobuf/gRPC usando Buf v2
proto-lint:
	@echo "Checking proto syntax with buf..."
	@buf lint

proto-gen:
	@echo "Generating Go code from protos..."
	@buf generate

migrate:
	atlas schema apply \
	  -u "$(DB_URL)" \
	  --to "file://$(SCHEMA_DIR)"



.PHONY: build,
		up,
		down,
		logs,
		bash,
		restart,
		unit,
		unit-verbose,
		unit-cover,
		unit-report,
		integration,
		proto-lint,
		proto-gen,
		migrate,
		air,
		run-race,
		test-load,
		help