# CUBELON 64 Makefile

# Tools
AS = ca65
LD = ld65
PYTHON = python3
VICE = x64sc

# Directories
SRC_DIR = src
BUILD_DIR = build
TOOLS_DIR = tools
DATA_DIR = $(SRC_DIR)/data
EFFECTS_DIR = $(SRC_DIR)/effects
INCLUDE_DIR = $(SRC_DIR)/include

# Output
TARGET = $(BUILD_DIR)/cubelon.prg
MAP = $(BUILD_DIR)/cubelon.map
LABEL = $(BUILD_DIR)/cubelon.lbl

# Assembler flags
ASFLAGS = -t c64 -I $(INCLUDE_DIR) -I $(SRC_DIR) -g

# Linker flags
LDFLAGS = -C c64.cfg -m $(MAP) -Ln $(LABEL)

# Source files (to be added as we create them)
SOURCES = $(SRC_DIR)/main.s \
          $(SRC_DIR)/display.s

# Object files
OBJECTS = $(SOURCES:$(SRC_DIR)/%.s=$(BUILD_DIR)/%.o)

# Default target
.PHONY: all
all: $(TARGET)

# Link
$(TARGET): $(OBJECTS) | $(BUILD_DIR)
	$(LD) $(LDFLAGS) -o $@ $(OBJECTS)
	@echo "Built: $@"

# Assemble
$(BUILD_DIR)/%.o: $(SRC_DIR)/%.s | $(BUILD_DIR)
	$(AS) $(ASFLAGS) -o $@ $<

# Create build directory
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# Run in VICE
.PHONY: run
run: $(TARGET)
	$(VICE) -autostartprgmode 1 $(TARGET)

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)/*

# Clean everything including generated data
.PHONY: distclean
distclean: clean
	rm -f $(DATA_DIR)/*.s
	rm -f $(TOOLS_DIR)/*.pyc
	rm -rf $(TOOLS_DIR)/__pycache__

# Help
.PHONY: help
help:
	@echo "CUBELON 64 - Makefile targets:"
	@echo "  all       - Build the demo (default)"
	@echo "  run       - Build and run in VICE"
	@echo "  clean     - Remove build artifacts"
	@echo "  distclean - Remove all generated files"
	@echo "  help      - Show this help message"
