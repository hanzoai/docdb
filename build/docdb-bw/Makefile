all: docdb_loader docdb

docdb_loader:
	$(MAKE) -C $@

docdb:
	$(MAKE) -C $@

clean:
	$(MAKE) -C docdb_loader clean
	$(MAKE) -C docdb clean

install:
	$(MAKE) -C docdb_loader install
	$(MAKE) -C docdb install

docker:
	docker buildx build --builder=docdb --tag=docdb-bw --load .
	docker run --platform=linux/amd64 --name=docdb-bw --rm -it \
		-e POSTGRES_USER=username -e POSTGRES_PASSWORD=password -e POSTGRES_DB=postgres \
		-v $(shell pwd)/../..:/docdb docdb-bw /bin/bash

docker-exec:
	docker exec -it docdb-bw /bin/bash

.PHONY: all docdb_loader docdb
