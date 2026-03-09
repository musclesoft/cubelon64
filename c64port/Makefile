.PHONY: all clean

TARGET = cubelon64
BUILD_DIR = build
SRC_DIR = src

SOURCES = $(SRC_DIR)/main.s

CA65 = ca65
LD65 = ld65
CA65FLAGS = -t c64 -I $(SRC_DIR)
LD65FLAGS = -C c64.cfg -m $(BUILD_DIR)/$(TARGET).map

all: $(BUILD_DIR)/$(TARGET).prg

$(BUILD_DIR)/$(TARGET).prg: $(SOURCES) c64.cfg | $(BUILD_DIR)
	$(CA65) $(CA65FLAGS) -o $(BUILD_DIR)/main.o $(SRC_DIR)/main.s
	$(LD65) $(LD65FLAGS) -o $@ $(BUILD_DIR)/main.o

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

clean:
	rm -rf $(BUILD_DIR)

run: $(BUILD_DIR)/$(TARGET).prg
	cd .. && go run ./emulator/run_cubelon.go -c64 c64port/$(BUILD_DIR)/$(TARGET).prg
