# Local development. CI runs the same checks (.github/workflows/ci.yml).

PG_CONTAINER ?= killy-pg
PG_PORT      ?= 55432
PG_URL       := postgres://postgres:killy@127.0.0.1:$(PG_PORT)/killy_test?sslmode=disable

.PHONY: test db-up db-down

# Runs every test, including those that need PostgreSQL. Start it first with make db-up.
test:
	go vet ./...
	KILLY_TEST_DATABASE_URL='$(PG_URL)' go test -race -count=1 ./...

# Starts a throwaway PostgreSQL in Docker for the tests (and for running killy server by hand
# with KILLY_DATABASE_URL set to the same URL).
db-up:
	docker run -d --rm --name $(PG_CONTAINER) -e POSTGRES_PASSWORD=killy -e POSTGRES_DB=killy_test \
		-p 127.0.0.1:$(PG_PORT):5432 postgres:16
	until docker exec $(PG_CONTAINER) pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done

db-down:
	docker stop $(PG_CONTAINER)
