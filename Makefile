demo-netting: CONFIG ?= demo/netting.local.yaml
demo-escrow: CONFIG ?= demo/escrow.local.yaml

.PHONY: demo-netting demo-escrow
demo-netting:
	cd backend && go run ./cmd/demo-netting --config ../$(CONFIG)

demo-escrow:
	cd backend && go run ./cmd/demo-escrow --config ../$(CONFIG)
