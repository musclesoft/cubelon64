# CUBELON Memory Map

## Summary

**Entry Point**: 0x7D00 (executed after all data loaded)
**Main Code Blocks**: data_10 (0xC006), data_11 (0xC007, Bank 2), data_12 (0xC408, Bank 3)

## Data Blocks and Memory Layout

| Block | File | Bank | Load Addr | Size | Content |
|-------|------|------|-----------|------|---------|
| 4 | data_01.bin | 0 | 0xC000 | 6144 | Screen bitmap |
| 5 | data_02.bin | 1 | 0x4000 | 7475 | Graphics (screen area) |
| 6 | data_03.bin | 6 | 0xC000 | 2494 | Data |
| 7 | data_04.bin | 0 | 0xC600 | 456 | Lookup tables |
| 8 | data_05.bin | 0 | 0x8800 | 1862 | Code/data |
| 9 | data_06.bin | 1 | 0xC600 | 125 | Data |
| 10 | data_07.bin | 2 | 0xC001 | 1530 | Data |
| 11 | data_08.bin | 0 | 0xD501 | 1116 | Data |
| 12 | data_09.bin | 0 | 0xC006 | 2005 | Data |
| 13 | data_10.bin | 0 | 0xC006 | 4594 | **Main code** |
| 14 | data_11.bin | 2 | 0xC007 | 11750 | **Major code/data** |
| 15 | data_12.bin | 3 | 0xC408 | 9445 | **Code** |

## Next Steps for Analysis

1. Search for 0x7D00 code in the data blocks
2. Disassemble data_05 at 0x8800 (may contain early init code)
3. Analyze data_10, data_11, data_12 for main demo code
4. Identify music player and effect routines
