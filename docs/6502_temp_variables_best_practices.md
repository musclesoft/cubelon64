# 6502 Temporary Variable Best Practices

## The Problem

In 6502 assembly with limited zero-page memory, temporary variables are shared across multiple functions. This creates **fragile dependencies** where one function can silently corrupt another's data.

## Real Bugs We Found

### Bug 1: Sign Flag Corruption
```asm
; mul16x16_signed32 stores result sign in math_temp+4
sta math_temp+4        ; Sign of result

; But then AL*BL multiply overwrites it!
sta math_temp+4        ; Temp storage ← OVERWRITES SIGN!
```

**Fix**: Don't reuse temp bytes that hold critical state.

### Bug 2: X Coordinate Destruction
```asm
; Store rotated X coordinate
sta math_temp          ; x' low byte
sta math_temp+1        ; x' high byte

; Call next multiplication - DESTROYS math_temp+0..3!
jsr mul16x16_signed32

; Try to use X coordinate - IT'S GONE!
lda math_temp          ; ← Corrupted!
```

**Fix**: Save X on stack before calculating Y.

### Bug 3: Saved Results Overwritten
```asm
; Save x*cos256 result
sta math_temp+4
; ...

; Call multiply again - overwrites math_temp+4!
jsr mul16x16_signed32  ; Uses math_temp+4 internally

; Try to retrieve saved value - CORRUPTED!
lda math_temp+4        ; ← Not what we saved!
```

**Fix**: Use stack instead of temp buffer between function calls.

## Best Practices

### 1. Document Lifetime and Ownership

```asm
; math_temp: 12 bytes shared temporary storage
;   VOLATILE - Contents destroyed by any math operation
;
;   During mul16x16_signed32:
;     Bytes 0-3:  32-bit result (OUTPUT)
;     Bytes 4-7:  Internal scratch (DESTROYED)
;     Bytes 8-11: Internal scratch (DESTROYED)
;
;   USER RULE: Save results immediately after function returns!
```

### 2. Use Stack for Preservation Across Calls

```asm
; ✓ GOOD: Save temp values on stack
lda math_temp+9
pha                    ; Save on stack
lda math_temp+10
pha
jsr another_function   ; math_temp now free for reuse
pla
sta result+1
pla
sta result

; ✗ BAD: Assume temp survives
lda math_temp+9
sta result            ; Store in another temp
jsr another_function  ; Might corrupt result!
lda result            ; May not be what you stored!
```

### 3. Function Contracts

Document what each function preserves/destroys:

```asm
; rotate_point - Rotate a 2D point
; ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
; Input:
;   temp1: X coordinate (16-bit Fixed 8.8)
;   temp2: Y coordinate (16-bit Fixed 8.8)
;   cos_val, sin_val: rotation values
;
; Output:
;   math_temp+0..1: Rotated X (16-bit)
;   math_temp+2..3: Rotated Y (16-bit)
;
; Destroys:
;   mul_a, mul_b: Used for multiplication
;   math_temp+0..11: ALL BYTES (internal computation)
;
; Preserves:
;   temp1, temp2: Input coordinates unchanged
;   cos_val, sin_val: Rotation values unchanged
;   corner variables: Not touched
;
; Stack usage: 12 bytes (4 saves × 3 operations)
```

### 4. Minimize Temp Lifetime

```asm
; ✓ GOOD: Short lifetime, immediate use
lda #value
sta temp
jsr process_temp       ; Uses temp immediately
; temp is now free for other uses

; ✗ BAD: Long lifetime across multiple calls
sta temp
jsr setup              ; Does this use temp?
jsr calculate          ; Does this use temp?
jsr finalize           ; Does this use temp?
lda temp               ; Is this still valid?
```

### 5. Prefer Dedicated Temp Spaces

```asm
; ✓ GOOD: Each subsystem has dedicated temps
.segment "ZEROPAGE"
rotation_temp:  .res 8    ; Only rotation.s uses these
render_temp:    .res 4    ; Only render.s uses these
math_scratch:   .res 12   ; Shared math operations

; ✗ BAD: One big shared temp for everything
math_temp: .res 12        ; Used by EVERYTHING
                          ; Who knows what survives?
```

### 6. Critical Section Annotations

```asm
; === CRITICAL: math_temp+0..3 valid here ===
lda math_temp
sta corner0_x
lda math_temp+1
sta corner0_x+1
; === END CRITICAL: math_temp now INVALID ===

; From this point, math_temp may be corrupted
jsr calculate_next     ; This WILL destroy math_temp
```

### 7. Static Analysis Comments

Add warnings at corruption points:

```asm
jsr mul16x16_signed32
; ⚠️  math_temp+0..11 now DESTROYED!
; ⚠️  Save any needed values BEFORE next operation!
lda math_temp          ; Only valid immediately after return
pha                    ; Save on stack NOW
```

### 8. Validation Patterns

For debugging, add validation:

```asm
; Store canary value in unused byte
lda #$AA
sta math_temp+11       ; Canary

; ... operations ...

lda math_temp+11       ; Check canary
cmp #$AA
beq @ok
; Canary corrupted - temp overflow or misuse!
```

## Summary Rules

1. **Never assume temp survives function calls**
2. **Save to stack before calling functions**
3. **Document what each function destroys**
4. **Minimize temp variable lifetime**
5. **Use dedicated temps when possible**
6. **Add critical section comments**
7. **Annotate corruption points**

## The Core Lesson

> In 6502 with limited zero-page space, temporary variables are both **precious** (fast, efficient) and **dangerous** (easily corrupted).
>
> **Treat them like hot potatoes**: grab, use immediately, and let go before calling anything else.
>
> **The stack is your friend** for preserving values across function boundaries.

## Tools to Help

1. **Memory map visualization**: Document all zero-page usage
2. **Call graph**: Show which functions call which
3. **Lifetime analysis**: Mark where each temp is valid
4. **Code review checklist**: Check all function calls for temp corruption

---

*This document created from hard-learned lessons during C64 rotation implementation.*
