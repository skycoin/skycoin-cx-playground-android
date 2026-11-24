.PHONY: bundle check-bundle test vet apk install emulator-image emulator-smoke collector

GOGIO_REF := 0magnet
APPID := com.skycoin.cxplayground
ARCH ?= arm64,amd64
APK ?= build/cx-playground.apk
ANDROID_HOME ?= /opt/android-sdk
ANDROID_NDK_ROOT ?= $(ANDROID_HOME)/ndk/30.0.16248370
# Where the benchmark suite app POSTs results. Empty disables upload.
BENCHMARK_UPLOAD_URL ?=
export ANDROID_HOME ANDROID_NDK_ROOT

bundle: ## Rebuild the embedded app bundle from apps/
	go run ./cmd/cxmanifest bundle -o cmd/cxplayground/bundle apps/* tests/*/app

check-bundle: ## Fail if the committed bundle is stale
	go run ./cmd/cxmanifest bundle -o build/bundle-check -signatures cmd/cxplayground/bundle apps/* tests/*/app > /dev/null
	diff -r build/bundle-check cmd/cxplayground/bundle

test: ## Run the tests, including every bundled CX app headless
	go test ./...
	go test -tags cxopcount ./tests/benchmark-opcounts

vet:
	go vet ./...

build/gogio/gogio:
	GOBIN=$(CURDIR)/build/gogio GOFLAGS= GOPROXY=direct go install github.com/0magnet/gio-cmd/gogio@$(GOGIO_REF)

apk: build/gogio/gogio check-bundle ## Build the APK for $(ARCH)
	GOFLAGS=-mod=vendor ./build/gogio/gogio -target android -arch $(ARCH) -minsdk 26 \
		-appid $(APPID) -name "CX Playground" \
		$(if $(BENCHMARK_UPLOAD_URL),-ldflags "-X main.benchmarkUploadURL=$(BENCHMARK_UPLOAD_URL)") \
		-o $(APK) ./cmd/cxplayground

install: apk ## Install the APK on the adb device and start it
	adb install -r $(APK)
	adb shell am start -n $(APPID)/org.gioui.GioActivity

RIG_IMAGE ?= cx-android-rig
# Extra driver flags, e.g. -container and the ports for a second rig.
EMULATOR_FLAGS ?=

emulator-image: ## Build the headless Android emulator image
	docker build -t $(RIG_IMAGE) docker/emulator

emulator-smoke: ## Boot the rig, build the APK, run every emulator test in tests/, stop the rig
	docker image inspect $(RIG_IMAGE) > /dev/null 2>&1 || $(MAKE) emulator-image
	go run ./cmd/emulator-driver suite -boot -stop -image $(RIG_IMAGE) $(EMULATOR_FLAGS)

collector: ## Build the benchmark results collector
	go build -o build/benchmark-collector ./cmd/benchmark-collector
