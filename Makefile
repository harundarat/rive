CONFIG ?= demo/netting.local.yaml

.PHONY: demo-netting
demo-netting:
	cd backend && go run ./cmd/demo-netting --config ../$(CONFIG)
