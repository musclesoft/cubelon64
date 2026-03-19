# Custom 6502 Assembler with Inline Assertions

A single-pass assembler for 6502 code with built-in assertion support for testing.

## Features

### Core Assembly
- ✅ All 6502 instructions with standard addressing modes
- ✅ Labels (global and local `@label`)
- ✅ Constants (`NAME = value`)
- ✅ Hex (`$FF`) and decimal literals
- ✅ Expressions: `+`, `-`, `*`, `/`, parentheses
- ✅ Low/high byte operators: `#<`, `#>`
- ✅ `.include` directive (inline file inclusion)
- ✅ `.import`/`.export` (tracked but not enforced)
- ✅ Segments: CODE, RODATA, DATA, BSS, ZEROPAGE, LOADADDR
- ✅ Data directives: `.byte`, `.word`, `.res`

### Assertions (New!)
- ✅ `.assert SYMBOL VALUE` - check memory equals value
- ✅ `.assert SYMBOL "file.bin" LENGTH [OFFSET]` - compare memory with file
- ✅ `.assert_if COND_SYMBOL COND_VALUE ...` - conditional assertions
- ✅ Generates `.prg.meta.json` with assertion metadata
- ✅ Position-based: assertions fire *before* next instruction

### Output
- ✅ C64 PRG format with load address
- ✅ Symbol table in metadata
- ✅ Assertion data for emulator integration

## Limitations

### Not Yet Supported
- ❌ Unnamed labels (`:+`, `:-`)
- ❌ `.proc`/`.endproc` scoping (ignored, but won't error)
- ❌ Macros
- ❌ Complex expression operators (`<<`, `>>`, `&`, `|`, `^`)
- ❌ `.ifdef`/`.ifndef` conditionals
- ❌ `.repeat` loops

### Workarounds
- **Multi-file projects**: Create a combined source file that includes all modules:
  ```asm
  .include "module1.s"
  .include "module2.s"
  .include "main.s"
  ```
- **Unnamed labels**: Use named local labels instead:
  ```asm
  ; Instead of: bcc :+
  ; Use:        bcc @skip
  ```

## Usage

```bash
./asm64 -C c64.cfg -o output.prg source.s
```

## Example with Assertions

```asm
.segment "CODE"

calc_rotation:
    ; ...compute corners...
    rts

; Check results when FRAME == 0
.assert_if FRAME 0 CORNER1_X "../golden/corners.bin" 8 0
    jsr render_square
```

This generates:
- `output.prg` - executable
- `output.prg.meta.json` - assertions + symbols

The emulator automatically loads the metadata and checks assertions during execution.

## Current Status

**Working**: Basic single-file assembly, all standard 6502 instructions, inline assertions, metadata generation.

**In Progress**: Full ca65 compatibility (unnamed labels, complex macros).

**Tested**: Simple programs assemble and run correctly. Assertions work in emulator.
