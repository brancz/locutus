DOCKER_IMAGE=quay.io/brancz/locutus:latest

.PHONY: docker
docker:
	@docker build -t $(DOCKER_IMAGE) .
