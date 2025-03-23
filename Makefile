.PHONY: all setup-go setup-python clean help

# Default target
all: setup-go setup-python
	@echo "✅ Environment setup complete"

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

# Clean up
clean:
	@echo "🧹 Cleaning up..."
	rm -rf venv
	@echo "✅ Cleanup complete"

# Help
help:
	@echo "Available targets:"
	@echo "  all           : Setup both Go and Python environments (default)"
	@echo "  setup-go      : Setup Go environment only"
	@echo "  setup-python  : Setup Python environment only"
	@echo "  clean         : Remove virtual environment and other generated files"
	@echo "  help          : Show this help message" 