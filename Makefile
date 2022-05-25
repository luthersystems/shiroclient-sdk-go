# Copyright © 2021 Luther Systems, Ltd. All right reserved.

# Makefile
#
# The primary project makefile that should be run from the root directory and is
# able to build and run the entire application.

PROJECT_REL_DIR=.
include ${PROJECT_REL_DIR}/common.mk

.DEFAULT_GOAL := default
.PHONY: default
default: all

.PHONY: all push clean

clean:
	rm -rf build

all: plugin
.PHONY: plugin plugin-linux plugin-darwin
plugin: ${SUBSTRATE_PLUGIN}

plugin-linux: ${SUBSTRATE_PLUGIN_LINUX}

plugin-darwin: ${SUBSTRATE_PLUGIN_DARWIN}

.PHONY: citest
citest: plugin test
	@

GO_TEST_BASE=${GO_HOST_EXTRA_ENV} SUBSTRATEHCP_FILE=${PWD}/${SUBSTRATE_PLUGIN_PLATFORM_TARGETED} go test ${GO_TEST_FLAGS}
GO_TEST_TIMEOUT_10=${GO_TEST_BASE} -timeout 10m

.PHONY: go-test
go-test:
	${GO_TEST_TIMEOUT_10} ./...

.PHONY: static-checks
static-checks: ${GO_PKG_DUMMY}
	./scripts/static-checks.sh

.PHONY: test
test: static-checks go-test
	@

${STATIC_PRESIGN_DUMMY}: ${LICENSE_FILE}
	${MKDIR_P} $(dir $@)
	./scripts/obtain-presigned.sh
	touch $@

${PRESIGNED_PATH}: ${STATIC_PRESIGN_DUMMY}
	@

${STATIC_PLUGINS_DUMMY}: ${PRESIGNED_PATH}
	${MKDIR_P} $(dir $@)
	./scripts/obtain-plugin.sh
	touch $@

${SUBSTRATE_PLUGIN}: ${STATIC_PLUGINS_DUMMY}
	@
