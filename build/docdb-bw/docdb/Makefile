GO ?= $(shell which go)
PG_CONFIG ?= pg_config
PG_INCLUDE := $(shell $(PG_CONFIG) --includedir-server)
PG_PKGLIB := $(shell $(PG_CONFIG) --pkglibdir)
INSTALL_BIN = $(shell which install)

CGO_CFLAGS ?= "-I$(PG_INCLUDE) -Wno-implicit-function-declaration"
CGO_LDFLAGS ?= "-shared"

object = docdb.so

all: $(object)

$(object): docdb.go
	CGO_CFLAGS=$(CGO_CFLAGS) CGO_LDFLAGS=$(CGO_LDFLAGS) $(GO) build -v -tags=docdb_bw -buildmode=c-shared -o $@ .

install: $(object)
	$(INSTALL_BIN) -c -m 755 $(object) $(PG_PKGLIB)

clean:
	rm -f $(object)
