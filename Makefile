# Binary poinhost-agent yang dibundel ke app desktop.
#
# CGO tidak dipakai (driver SQLite-nya modernc.org/sqlite, murni Go), jadi
# cross-compile ke arm64 tidak butuh toolchain C sama sekali — dan perintah
# yang sama juga jalan di runner CI Windows maupun macOS.
VERSION ?= dev
AGENT_DIR := internal/agent/dist/bin
LDFLAGS := -s -w -X main.Version=$(VERSION)

# Prasyarat sumber. TANPA ini, target .gz yang sudah ada dianggap selalu
# mutakhir dan `make agent` jadi no-op diam-diam — persis yang membuat binary
# basah di-embed ke app desktop lalu terpasang ke server, tanpa satu pun
# tanda bahwa ia tertinggal dari source.
AGENT_SRC := $(shell find cmd/poinhost-agent internal -name '*.go' -not -name '*_test.go' 2>/dev/null)

.PHONY: agent agent-clean agent-rebuild

agent: $(AGENT_DIR)/poinhost-agent-linux-amd64.gz $(AGENT_DIR)/poinhost-agent-linux-arm64.gz

$(AGENT_DIR)/poinhost-agent-linux-%.gz: $(AGENT_SRC) Makefile
	@mkdir -p $(AGENT_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=$* go build -ldflags="$(LDFLAGS)" -o $(AGENT_DIR)/poinhost-agent-linux-$* ./cmd/poinhost-agent
	gzip -9 -f $(AGENT_DIR)/poinhost-agent-linux-$*
	@printf '%s' "$(VERSION)" > $(AGENT_DIR)/version.txt
	@ls -lh $@

# Paksa bangun ulang tanpa memikirkan timestamp.
agent-rebuild: agent-clean agent

agent-clean:
	rm -f $(AGENT_DIR)/*.gz $(AGENT_DIR)/version.txt
