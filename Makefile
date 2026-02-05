ALL_MODULES := $(shell find . -type f -name "go.mod" -exec dirname {} \; | sort )

.PHONY: for-all
for-all:
	@set -e; for dir in $(ALL_MODULES); do \
	  (cd "$${dir}" && \
	  	echo "running $${CMD} in $${dir}" && \
	 	$${CMD} ); \
	done

.PHONY: tidy
tidy:
	$(MAKE) for-all CMD="go mod tidy -compat=1.24"
