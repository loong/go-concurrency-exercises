.PHONY: server client

ADDR ?= :8080
CAPACITY ?= 200
WORKERS ?= 8
TOTAL ?= 100
CONCURRENCY ?= 4
ITERATIONS ?= 3
PPROF_PORT ?= 8081
PPROF_FILE ?= mem.pprof

server:
	go run main.go -mode=server -addr=$(ADDR) -capacity=$(CAPACITY) -workers=$(WORKERS)

client:
	go run main.go -mode=client -addr=$(ADDR) -total=$(TOTAL) -concurrency=$(CONCURRENCY) -iterations=$(ITERATIONS)

profmem:
	go tool pprof -http=:$(PPROF_PORT) $(PPROF_FILE)
