FLAGS ?= -v

test:
	go test $(FLAGS) ./...

test-race:
	go test $(FLAGS) -race ./...
