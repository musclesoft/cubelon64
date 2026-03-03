# CUBELON TAP File Extraction

## Original File
- **File**: original/cubelon_final.tap (48 KB)
- **Format**: ZX Spectrum TAP tape image
- **Program**: CUBELON

## BASIC Loader Program

```basic
10 CLEAR 24575
20 REM "" LOAD
30 RANDOMIZE USR 30720
```

The loader:
1. Sets memory boundary at 24575
2. Calls machine code at address 30720 (0x7800)

## Extracted Files

All extracted files are located in the `original/` folder:

### Core Files
- `original/cubelon_basic.bin` (45 bytes) - BASIC program
- `original/cubelon_code.bin` (139 bytes) - Initial machine code at address 30720

### Data Blocks
- `original/cubelon_data_01.bin` (6,145 bytes)
- `original/cubelon_data_02.bin` (7,476 bytes)
- `original/cubelon_data_03.bin` (2,495 bytes)
- `original/cubelon_data_04.bin` (457 bytes)
- `original/cubelon_data_05.bin` (1,863 bytes)
- `original/cubelon_data_06.bin` (126 bytes)
- `original/cubelon_data_07.bin` (1,531 bytes)
- `original/cubelon_data_08.bin` (1,117 bytes)
- `original/cubelon_data_09.bin` (2,006 bytes)
- `original/cubelon_data_10.bin` (4,595 bytes)
- `original/cubelon_data_11.bin` (11,751 bytes)
- `original/cubelon_data_12.bin` (9,446 bytes)

### Combined File
- `original/cubelon_all_data.bin` (49,147 bytes) - All data blocks combined including machine code

## Total Extracted Data
49,147 bytes of program data across 12 data blocks
