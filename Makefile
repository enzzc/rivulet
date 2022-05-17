all: rivulet image

rivulet: $(wildcard *.go)
	CGO_ENABLED=0 go build -o rivulet -v .

.PHONY: fmt
fmt:
	go fmt .

.PHONY: run
run:
	go run .

image: rivulet
	podman build . -t rivulet

.PHONY: push
push: image
	podman tag rivulet $(USER)/rivulet:latest
	podman push $(USER)/rivulet:latest docker.io/$(USER)/rivulet --creds=$(USER)

.PHONY: rollout
rollout: push
	kubectl rollout restart deployment/rivulet