FLAGS ?= -v

test:
	go test $(FLAGS) ./...
