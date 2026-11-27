# Local development. CI runs the same checks (.github/workflows/ci.yml).

PG_CONTAINER ?= killy-pg
PG_PORT      ?= 55432

.PHONY: test dbcheck e2e db-up db-down

# Unit tests only; none of them needs PostgreSQL.
test:
	go vet ./...
	go test -race -count=1 ./...

# Runs killy server against the PostgreSQL that `make db-up` started: applies the migrations, pairs, restarts.
# It writes an agent and a device to that database, so use a throwaway one.
dbcheck:
	KILLY_DATABASE_URL='postgres://postgres:killy@127.0.0.1:$(PG_PORT)/killy?sslmode=disable' tools/dbcheck/run.sh

# Runs a real agent and zombie parent against the same PostgreSQL and checks kills end to end. Linux only.
e2e:
	KILLY_DATABASE_URL='postgres://postgres:killy@127.0.0.1:$(PG_PORT)/killy?sslmode=disable' tools/e2e/run.sh

# Starts a throwaway PostgreSQL 18.7 in Docker for running killy server by hand, with
# KILLY_DATABASE_URL=postgres://postgres:killy@127.0.0.1:55432/killy?sslmode=disable
db-up:
	docker run -d --rm --name $(PG_CONTAINER) -e POSTGRES_PASSWORD=killy -e POSTGRES_DB=killy \
		-p 127.0.0.1:$(PG_PORT):5432 postgres:18.7
	until docker exec $(PG_CONTAINER) pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done

db-down:
	docker stop $(PG_CONTAINER)
