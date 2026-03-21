.PHONY: all clean new

# NOTE: This Makefile uses the legacy ca65/ld65 toolchain.
# The new custom assembler (../asm/asm64) with inline assertions is preferred.
# Use 'make new' to build with the new assembler (produces identical output to ca65).

TARGET = cubelon64
BUILD_DIR = build
SRC_DIR = src

SOURCES = $(SRC_DIR)/main.s $(SRC_DIR)/rotation.s $(SRC_DIR)/charset.s $(SRC_DIR)/render.s $(SRC_DIR)/math_helpers.s $(SRC_DIR)/sincos_tables.s $(SRC_DIR)/bit_reverse_table.s $(SRC_DIR)/animate.s $(SRC_DIR)/sequencer.s

CA65 = ca65
LD65 = ld65
CA65FLAGS = -t c64 -I $(SRC_DIR)
LD65FLAGS = -C c64.cfg -m $(BUILD_DIR)/$(TARGET).map --force-import test_checkpoint_frame --force-import PRECOMPUTED_FRAMES --force-import FRAME --force-import test_after_calc_rotation --force-import test_after_render --force-import SPRITE_DATA --force-import downsample_spans --force-import get_scanline_span

all: $(BUILD_DIR)/$(TARGET).prg

$(BUILD_DIR)/$(TARGET).prg: $(SOURCES) c64.cfg | $(BUILD_DIR)
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/main.o $(SRC_DIR)/main.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/rotation.o $(SRC_DIR)/rotation.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/charset.o $(SRC_DIR)/charset.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/render.o $(SRC_DIR)/render.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/math_helpers.o $(SRC_DIR)/math_helpers.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/sincos_tables.o $(SRC_DIR)/sincos_tables.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/bit_reverse_table.o $(SRC_DIR)/bit_reverse_table.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/animate.o $(SRC_DIR)/animate.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/sequencer.o $(SRC_DIR)/sequencer.s
	$(LD65) $(LD65FLAGS) -o $@ $(BUILD_DIR)/main.o $(BUILD_DIR)/rotation.o $(BUILD_DIR)/charset.o $(BUILD_DIR)/render.o $(BUILD_DIR)/math_helpers.o $(BUILD_DIR)/sincos_tables.o $(BUILD_DIR)/bit_reverse_table.o $(BUILD_DIR)/animate.o $(BUILD_DIR)/sequencer.o

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

clean:
	rm -rf $(BUILD_DIR)

run: $(BUILD_DIR)/$(TARGET).prg
	go run -C ../emulator run_cubelon.go assertions.go motion_analyzer.go watchpoint_trace.go -c64 ../c64port/$(BUILD_DIR)/$(TARGET).prg

# New assembler with inline assertions (produces identical output to ca65)
new: $(SRC_DIR)/all.s | $(BUILD_DIR)
	go run -C ../asm . -C ../c64port/c64.cfg -o ../c64port/$(BUILD_DIR)/$(TARGET).prg ../c64port/$(SRC_DIR)/all.s
