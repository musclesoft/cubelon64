.PHONY: all clean

TARGET = cubelon64
BUILD_DIR = build
SRC_DIR = src

SOURCES = $(SRC_DIR)/main.s $(SRC_DIR)/rotation.s $(SRC_DIR)/charset.s $(SRC_DIR)/render.s $(SRC_DIR)/math_helpers.s $(SRC_DIR)/sincos_tables.s $(SRC_DIR)/animate.s

CA65 = ca65
LD65 = ld65
CA65FLAGS = -t c64 -I $(SRC_DIR)
LD65FLAGS = -C c64.cfg -m $(BUILD_DIR)/$(TARGET).map --force-import test_checkpoint_frame --force-import test_copy_frame0 --force-import PRECOMPUTED_FRAMES --force-import FRAME --force-import test_after_calc_rotation --force-import test_after_render

all: $(BUILD_DIR)/$(TARGET).prg

$(BUILD_DIR)/$(TARGET).prg: $(SOURCES) c64.cfg | $(BUILD_DIR)
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/main.o $(SRC_DIR)/main.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/rotation.o $(SRC_DIR)/rotation.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/charset.o $(SRC_DIR)/charset.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/render.o $(SRC_DIR)/render.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/math_helpers.o $(SRC_DIR)/math_helpers.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/sincos_tables.o $(SRC_DIR)/sincos_tables.s
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/animate.o $(SRC_DIR)/animate.s
	$(LD65) $(LD65FLAGS) -o $@ $(BUILD_DIR)/main.o $(BUILD_DIR)/rotation.o $(BUILD_DIR)/charset.o $(BUILD_DIR)/render.o $(BUILD_DIR)/math_helpers.o $(BUILD_DIR)/sincos_tables.o $(BUILD_DIR)/animate.o

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

clean:
	rm -rf $(BUILD_DIR)

run: $(BUILD_DIR)/$(TARGET).prg
	cd .. && go run ./emulator/run_cubelon.go -c64 c64port/$(BUILD_DIR)/$(TARGET).prg
