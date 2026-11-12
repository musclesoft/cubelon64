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
- ✅ `.align no_cross_page_branch` directly before a `.proc`: pads the proc by the fewest bytes (0 to 255) that keep every branch inside it from crossing a page, since a taken branch to another page costs an extra cycle. Branches to labels outside the proc are not checked. Pass 2 fails if a marked proc still has a crossing branch.

### Assertions (New!)

- ✅ `.assert SYMBOL VALUE` - check memory equals value
- ✅ `.assert SYMBOL "file.bin" LENGTH [OFFSET]` - compare memory with file
- ✅ `.assert_if COND_SYMBOL COND_VALUE ...` - conditional assertions
- ✅ `assert_flag FLAG, VALUE` - the next instruction must run with status flag C, Z, I, D, V or N at 0 or 1 (an empty macro for ca65, like `assert_if`)
- ✅ Generates `.prg.meta.json` with assertion metadata
- ✅ Position-based: assertions fire _before_ next instruction

### Output

- ✅ C64 PRG format with load address
- ✅ Symbol table in metadata
- ✅ Assertion data for emulator integration
- ✅ Parameterless `.macro`/`.endmacro`, expanded textually at each use with `@` labels scoped per expansion

## Limitations

### Not Yet Supported

- ❌ Unnamed labels (`:+`, `:-`)
- ❌ `.proc`/`.endproc` scoping (ignored, but won't error)
- ❌ `.macro` parameters (parameterless macros work)
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

**Assemble a program:**

```bash
go run -C asm . -o output.prg -C config.cfg source.s
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

**Working**: Two-pass assembly with segment-relative addressing, all standard 6502 instructions, inline assertions, metadata generation. Produces byte-for-byte identical output to ca65 for the C64 port.

**Not Yet Implemented**: Unnamed labels (`:+`, `:-`), macro parameters, complex expression operators, conditionals, `.repeat` loops.

**Tested**: C64 port assembles correctly. Binary output verified identical to ca65. Assertions work in emulator.
