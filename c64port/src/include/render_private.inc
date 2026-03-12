; render_private.inc - Private variables for render.s ONLY
; ABSOLUTE ADDRESSING ONLY - No zero page, No temps
; Base address: $0250

; ═══════════════════════════════════════════════════════════
; RENDER MODULE PRIVATE - Do NOT use in other modules
; ═══════════════════════════════════════════════════════════

; Scanline state
RND_SCANLINE_Y = $0250           ; Current scanline being processed (0-23)
RND_SPAN_X1 = $0251              ; Span start X
RND_SPAN_X2 = $0252              ; Span end X
RND_INTERSECTION_COUNT = $0253   ; Number of edge intersections found

; Edge being processed
RND_EDGE_X0 = $0254              ; Edge start X coordinate
RND_EDGE_X1 = $0255              ; Edge end X coordinate
RND_EDGE_Y0 = $0256              ; Edge start Y coordinate
RND_EDGE_Y1 = $0257              ; Edge end Y coordinate

; Edge calculation results
RND_DX_ABS = $0258               ; Absolute value of (X1 - X0)
RND_DY_ABS = $0259               ; Absolute value of (Y1 - Y0)
RND_DX_SIGN = $025A              ; Sign of dX (0=positive, 1=negative)
RND_STEP_Y = $025B               ; Step along edge (always positive)

; mul_8bit function (private, only used by mul_8bit)
MUL8_INPUT_A = $025C             ; 8-bit multiplicand
MUL8_INPUT_B = $025D             ; 8-bit multiplier
MUL8_RESULT_LO = $025E           ; Result low byte
MUL8_RESULT_HI = $025F           ; Result high byte
MUL8_COUNTER = $0260             ; Bit counter for shift-and-add

; div16by8 function (private, only used by div16by8)
DIV16_DIVIDEND_LO = $0261        ; 16-bit dividend low byte
DIV16_DIVIDEND_HI = $0262        ; 16-bit dividend high byte
DIV16_DIVISOR = $0263            ; 8-bit divisor
DIV16_REMAINDER = $0264          ; Remainder accumulator
DIV16_QUOTIENT = $0265           ; Quotient result (low 8 bits)

; draw_span function (private)
SPAN_BITMASK_COL0 = $0266        ; Bitmask for column 0
SPAN_BITMASK_COL1 = $0267        ; Bitmask for column 1
SPAN_BITMASK_COL2 = $0268        ; Bitmask for column 2
SPAN_PIXEL_X = $0269             ; Current pixel X position
SPAN_SAVED_Y = $026A             ; Saved scanline Y value

; DDA edge walking state (4 edges × 7 bytes = 28 bytes)
; Edge 0 (corner0 -> corner1)
EDGE0_X_CURRENT = $026B          ; Current X position
EDGE0_ERROR = $026C              ; Bresenham error accumulator
EDGE0_DX_ABS = $026D             ; |X1 - X0|
EDGE0_DY_ABS = $026E             ; |Y1 - Y0|
EDGE0_DX_SIGN = $026F            ; Sign of dX (0=positive, 1=negative)
EDGE0_Y_START = $0270            ; First scanline (min Y)
EDGE0_Y_END = $0271              ; Last scanline (max Y)

; Edge 1 (corner1 -> corner2)
EDGE1_X_CURRENT = $0272
EDGE1_ERROR = $0273
EDGE1_DX_ABS = $0274
EDGE1_DY_ABS = $0275
EDGE1_DX_SIGN = $0276
EDGE1_Y_START = $0277
EDGE1_Y_END = $0278

; Edge 2 (corner2 -> corner3)
EDGE2_X_CURRENT = $0279
EDGE2_ERROR = $027A
EDGE2_DX_ABS = $027B
EDGE2_DY_ABS = $027C
EDGE2_DX_SIGN = $027D
EDGE2_Y_START = $027E
EDGE2_Y_END = $027F

; Edge 3 (corner3 -> corner0)
EDGE3_X_CURRENT = $0280
EDGE3_ERROR = $0281
EDGE3_DX_ABS = $0282
EDGE3_DY_ABS = $0283
EDGE3_DX_SIGN = $0284
EDGE3_Y_START = $0285
EDGE3_Y_END = $0286

; Bufferless supersampling - stores span state for 2 supersample rows
SPAN_ROW0_X1 = $0287          ; Row 0 span start X (0-47)
SPAN_ROW0_X2 = $0288          ; Row 0 span end X (0-47)
SPAN_ROW1_X1 = $0289          ; Row 1 span start X (0-47)
SPAN_ROW1_X2 = $028A          ; Row 1 span end X (0-47)
SPAN_ROW0_VALID = $028B       ; Row 0 has valid span (0/1)
SPAN_ROW1_VALID = $028C       ; Row 1 has valid span (0/1)

; Downsampling state
DOWNSAMPLE_Y = $028D          ; Current output Y being processed
DOWNSAMPLE_X = $028E          ; Current output X being processed
SUBPIXEL_COUNT = $028F        ; Count of set subpixels (0-4)

; Corner backup (for preserving original values before 2× scaling)
CORNER_BACKUP = $0290         ; 16 bytes to backup all 8 corner coordinates

; Optimized render variables (added for render_optimized.s)
SPAN_PIXEL_X1 = $02A0         ; ssX + 1 (supersample coordinate)
SPAN_WRAP0 = $02A1            ; Row 0 wraps flag (0/1)
SPAN_WRAP1 = $02A2            ; Row 1 wraps flag (0/1)
SPAN_ROW0_X2_WRAP = $02A3     ; Row 0 wrapped X2 (x2-48)
SPAN_ROW1_X2_WRAP = $02A4     ; Row 1 wrapped X2 (x2-48)
SPAN_TEMP = $02A5             ; Temporary storage for inverted mask
