.PHONY: all setup-go setup-python clean help test-go build install

# Default target
all: setup-go setup-python
	@echo "✅ Environment setup complete"

test-go:
	@echo "🧪 Running Go tests..."
	go test ./...
	@echo "✅ Go tests completed"

# Setup Go environment
setup-go:
	@echo "📦 Setting up Go environment..."
	go get golang.org/x/tools/go/packages
	@echo "✅ Go environment setup complete"

# Setup Python environment
setup-python:
	@echo "🐍 Setting up Python environment..."
	python3 -m venv venv
	. venv/bin/activate && pip install --upgrade pip && pip install -r requirements.txt
	@echo "✅ Python environment setup complete"

# Build the Go binary locally
build:
	@echo "🔨 Building GoSrcCtrl executable..."
	go build -o gosrctrl main.go
	@echo "✅ Build complete. You can run './gosrctrl' locally."

# Install globally (places the executable in GOPATH/bin or GOBIN)
install:
	@echo "🚀 Installing GoSrcCtrl globally..."
	go install
	@echo "✅ Installation complete. Make sure GOPATH/bin or GOBIN is on your PATH, then run 'gosrctrl' from anywhere."

# Clean up
clean:
	@echo "🧹 Cleaning up..."
	rm -rf venv
	rm -f gosrctrl
	@echo "✅ Cleanup complete"

# Help
help:
	@echo "Available targets:"
	@echo "  all           : Setup both Go and Python environments (default)"
	@echo "  setup-go      : Setup Go environment only"
	@echo "  setup-python  : Setup Python environment only"
	@echo "  test-go       : Run Go tests"
	@echo "  build         : Compile the GoSrcCtrl binary locally"
	@echo "  install       : Install the GoSrcCtrl binary to GOPATH/bin or GOBIN"
	@echo "  clean         : Remove virtual environment, build artifacts, etc."
	@echo "  help          : Show this help message" 