# Migration to Custom Assembler

## Status: In Progress

### ✅ Completed

1. **Removed Old Test Infrastructure**
   - ❌ `c64port/test_config.json` - deleted (replaced by inline `.assert`)
   - ❌ `emulator/test_runner.go` - deleted (integrated into `run_cubelon.go`)

2. **Replaced Unnamed Labels**
   - All `:+` and `:-` labels converted to named local labels
   - Example: `:+` → `@no_carry1`, `@no_carry2`
   - Codebase now compatible with custom assembler syntax

3. **Built Custom Assembler**
   - Location: `asm/asm`
   - Features: inline assertions, metadata generation
   - See `asm/README.md` for details

4. **Integrated Assertions in Emulator**
   - Loads `.prg.meta.json` automatically
   - Checks assertions during execution
   - See `emulator/assertions.go`

### ⚠️ In Progress

**Custom Assembler Compatibility**
- ✅ Single-file assembly works
- ✅ Basic `.include` support
- ⚠️ Full multi-file project: branch offset issues remain

**Current Blocker:**
- Some branches exceed 127-byte relative offset limit
- Need to either:
  - Use absolute jumps for far targets
  - Add `.jcc` macro support (conditional absolute jump)
  - Reorganize code to reduce distances

### 🔄 Build Options

**Legacy (Current Default):**
```bash
cd c64port
make                    # Uses ca65/ld65
make run                # Build and run in emulator
```

**New Assembler:**
```bash
make -C c64port new     # Uses custom assembler
```

Or directly:
```bash
go run -C asm . -C c64port/c64.cfg -o c64port/build/output.prg c64port/src/all.s
```

### 📋 TODO: Complete Migration

1. **Fix Branch Offsets**
   - Option A: Change long branches to `jmp` instructions
   - Option B: Add macro support for smart branch/jump selection
   - Option C: Reorganize code layout

2. **Add Missing Features** (if needed)
   - Complex expression operators (`<<`, `>>`, `&`, `|`)
   - `.ifdef`/`.ifndef` conditionals
   - Macro system

3. **Add Inline Assertions**
   - Port assertions from deleted `test_config.json` to inline `.assert` statements
   - Examples in `animate.s` and `main.s` (currently commented out)

4. **Delete Legacy Build**
   - Remove `Makefile` dependency on ca65/ld65
   - Update docs to reference new assembler only

### 🎯 Goal

Replace:
```
Edit asm → ca65 → ld65 → load JSON config → emulator checks external assertions
```

With:
```
Edit asm (with inline .assert) → asm → emulator auto-checks embedded assertions
```

**Benefits:**
- ✅ Faster iteration (one build step)
- ✅ Self-documenting tests (assertions in source)
- ✅ No external toolchain dependency
- ✅ Richer assertion metadata
