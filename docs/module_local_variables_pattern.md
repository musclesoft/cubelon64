# Module-Local Variables in 6502 Assembly

## The Problem with Global Shared Temps

Our current architecture has **everything in global zero page**:

```asm
; main.s - EVERYTHING is exported globally!
.exportzp corner0_x, corner0_y, corner1_x, corner1_y  ; OK - truly shared
.exportzp temp1, temp2, temp3                         ; BAD - used locally
.exportzp mul_a, mul_b, mul_result, math_temp         ; BAD - internal to rotation.s
.exportzp scan_y, scan_x1, scan_x2                    ; BAD - internal to render.s
```

**Result**: Every module can corrupt every other module's data!

## Better Architecture: Module-Local Variables

### Pattern 1: Private Zero Page Variables

```asm
; rotation.s - Keep internal variables PRIVATE
.segment "ZEROPAGE"

; Module-private - NOT exported, only visible in this file
rotation_temp1: .res 2
rotation_temp2: .res 2
mul_scratch: .res 12      ; Only for internal math

; These DO need to be exported (used by caller)
.exportzp corner0_x, corner0_y
corner0_x: .res 2
corner0_y: .res 2
```

### Pattern 2: Function-Local Stack Allocation

```asm
; Instead of global temps, use stack frame
calc_rotation:
    ; Allocate 8 bytes of local storage on stack
    tsx
    txa
    sec
    sbc #8
    tax
    txs
    ; Now use 0,X 1,X 2,X etc. as local variables

    ; ... do work ...

    ; Deallocate on exit
    tsx
    txa
    clc
    adc #8
    tax
    txs
    rts
```

### Pattern 3: Explicit Ownership Documentation

```asm
; zeropage.inc - Document ownership clearly

.segment "ZEROPAGE"

; ═══ SHARED - Multiple modules ═══
.exportzp corner0_x, corner0_y, corner1_x, corner1_y
corner0_x: .res 2
corner0_y: .res 2
corner1_x: .res 2
corner1_y: .res 2

.exportzp cos_val, sin_val
cos_val: .res 2           ; OWNER: rotation.s (read-only for others)
sin_val: .res 2

; ═══ rotation.s MODULE ═══
.exportzp rotation_mul_a, rotation_mul_b
rotation_mul_a: .res 2    ; PRIVATE to rotation.s
rotation_mul_b: .res 2
rotation_scratch: .res 12 ; PRIVATE to rotation.s

; ═══ render.s MODULE ═══
.exportzp render_scan_y, render_scan_x1
render_scan_y: .res 1     ; PRIVATE to render.s
render_scan_x1: .res 1
render_scratch: .res 4    ; PRIVATE to render.s
```

## Recommended Architecture

### Structure:

```
c64port/
├── src/
│   ├── main.s           # Entry point, initialization
│   ├── include/
│   │   ├── globals.inc  # ONLY truly shared variables
│   │   └── modules.inc  # Module exports (functions only)
│   ├── rotation.s       # Has private zero page section
│   ├── render.s         # Has private zero page section
│   └── ...
```

### globals.inc - Minimal Shared State

```asm
; globals.inc - ONLY variables shared between modules

.segment "ZEROPAGE"

; ═══ SHARED STATE ═══
; Corner coordinates - written by rotation.s, read by render.s
.exportzp corner0_x, corner0_y, corner1_x, corner1_y
.exportzp corner2_x, corner2_y, corner3_x, corner3_y
corner0_x: .res 2
corner0_y: .res 2
corner1_x: .res 2
corner1_y: .res 2
corner2_x: .res 2
corner2_y: .res 2
corner3_x: .res 2
corner3_y: .res 2

; Rotation parameters - set by main, used by rotation.s
.exportzp angle, frame
angle: .res 1
frame: .res 1

; NO TEMP VARIABLES HERE!
```

### rotation.s - Module-Private Variables

```asm
; rotation.s - Rotation calculations with PRIVATE variables

.include "globals.inc"      ; Get shared state

; ═══ MODULE-PRIVATE ZERO PAGE ═══
.segment "ZEROPAGE"

; These are NOT exported - only visible in rotation.s
rot_temp1: .res 2          ; Private temp for calculations
rot_temp2: .res 2
rot_mul_a: .res 2          ; Private multiply operands
rot_mul_b: .res 2
rot_result: .res 4         ; Private result buffer
rot_scratch: .res 12       ; Private scratch space

; Internal trig values (computed once, used internally)
rot_cos256: .res 2
rot_sin256: .res 2

; ═══ EXPORTED FUNCTIONS ═══
.segment "CODE"

; Public API - the ONLY thing other modules see
.export calc_rotation

calc_rotation:
    ; Use rot_temp1, rot_temp2, etc.
    ; Other modules CAN'T corrupt these!
    ; ...
    rts

; Private helper - not exported
mul16x16_signed:
    ; Uses rot_mul_a, rot_mul_b, rot_result
    ; Completely isolated from other modules
    rts
```

### render.s - Module-Private Variables

```asm
; render.s - Rendering with PRIVATE variables

.include "globals.inc"      ; Get corner coordinates

; ═══ MODULE-PRIVATE ZERO PAGE ═══
.segment "ZEROPAGE"

; Private to render.s
rnd_scan_y: .res 1
rnd_scan_x1: .res 1
rnd_scan_x2: .res 1
rnd_edge_count: .res 1
rnd_temp: .res 4

; ═══ EXPORTED FUNCTIONS ═══
.segment "CODE"

.export render_square

render_square:
    ; Uses rnd_scan_y, rnd_temp, etc.
    ; rotation.s can't corrupt these!
    ; ...
    rts
```

## Migration Strategy

### Step 1: Identify True Shared State
```asm
# What REALLY needs to be shared?
- corner coordinates (rotation → render)
- angle (main → rotation)
- frame counter (main → rotation)

# What is actually private?
- temp1, temp2 (only rotation.s)
- mul_a, mul_b (only rotation.s)
- scan_y, scan_x1 (only render.s)
```

### Step 2: Create Module-Private Sections

Each module gets its own zero page segment:

```asm
; rotation.s
.segment "ZEROPAGE" : zeropage
.scope Rotation
    temp1: .res 2     ; Accessed as Rotation::temp1
    temp2: .res 2
.endscope
```

### Step 3: Update References

```asm
; Before:
lda temp1          ; Global temp1
sta corner0_x

; After:
lda Rotation::temp1    ; Scoped to rotation module
sta corner0_x
```

## Benefits

### 1. **Isolation**
```asm
; render.s can't accidentally corrupt rotation variables
; Each module has a "firewall" around its internals
```

### 2. **Clear Contracts**
```asm
; Interface is explicit - only exported symbols are visible
; If it's not in the .export list, you can't use it!
```

### 3. **Easier Debugging**
```asm
; If rot_temp1 is corrupted, only rotation.s could have done it
; Narrows the search space dramatically
```

### 4. **Safe Refactoring**
```asm
; Can change internal implementation without breaking other modules
; As long as the public API stays the same
```

## Zero Page Allocation Strategy

Zero page is precious (256 bytes). Allocate strategically:

```asm
$00-$01: Reserved (used by KERNAL)
$02-$3F: Our program (62 bytes)
    $02-$11: Shared state (16 bytes)
        - 8 corners × 2 bytes = 16 bytes
    $12-$1F: rotation.s private (14 bytes)
    $20-$2F: render.s private (16 bytes)
    $30-$3F: main.s private (16 bytes)
$40-$FF: Stack and system use
```

## Example: Fixed Rotation Module

```asm
; rotation.s - Properly isolated version

.include "include/globals.inc"

; ═══ PRIVATE ZERO PAGE ═══
.segment "ROTATIONZP": zeropage

rot_temp_x: .res 2      ; Temp for X calculation
rot_temp_y: .res 2      ; Temp for Y calculation
rot_mul_a: .res 2       ; Multiply input A
rot_mul_b: .res 2       ; Multiply input B
rot_result32: .res 4    ; 32-bit multiply result
rot_scratch: .res 8     ; Computation scratch space

; ═══ CODE ═══
.segment "CODE"

.export calc_rotation

calc_rotation:
    ; Store cos256/sin256 in private variables
    lda #$B5
    sta rot_cos256
    sta rot_sin256
    lda #$00
    sta rot_cos256+1
    sta rot_sin256+1

    ; Calculate corner 0
    lda #$00
    sta rot_temp_x
    lda #$F4
    sta rot_temp_x+1      ; X = $F400

    jsr rotate_point      ; Uses private variables!

    ; Results in rot_result32
    ; Copy to shared corner0_x/y
    lda rot_result32
    sta corner0_x
    lda rot_result32+1
    sta corner0_x+1
    ; ...

    rts

; PRIVATE function - not exported
rotate_point:
    ; All operations use rot_* variables
    ; Completely isolated from render.s!
    ; ...
    rts
```

## Summary

| Pattern | When to Use | Benefits |
|---------|-------------|----------|
| **Module-private zero page** | Internal temps, scratch space | Isolation, safety |
| **Stack-based locals** | Function temps, recursion | Maximum isolation |
| **Explicit shared globals** | True cross-module state | Clear contracts |

**Golden Rule**: Export only what MUST be shared. Everything else stays private!

---

*This pattern would have prevented all 3 temp variable bugs we encountered.*
