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

; init_edge_with_div temp storage (16-bit Fixed88 processing)
INIT_EDGE_X0_LO = $02D4          ; Temp corner 0 X lo
INIT_EDGE_X0_HI = $02D5          ; Temp corner 0 X hi
INIT_EDGE_Y0_LO = $02D6          ; Temp corner 0 Y lo
INIT_EDGE_Y0_HI = $02D7          ; Temp corner 0 Y hi
INIT_EDGE_X1_LO = $02D8          ; Temp corner 1 X lo
INIT_EDGE_X1_HI = $02D9          ; Temp corner 1 X hi
INIT_EDGE_Y1_LO = $02DA          ; Temp corner 1 Y lo
INIT_EDGE_Y1_HI = $02DB          ; Temp corner 1 Y hi
INIT_EDGE_DX_LO = $02DC          ; Temp dx magnitude lo
INIT_EDGE_DX_HI = $02DD          ; Temp dx magnitude hi
INIT_EDGE_YSTART = $02DE         ; Temp yStart
INIT_EDGE_YEND = $02DF           ; Temp yEnd
INIT_EDGE_DXSIGN = $02E0         ; Temp dx sign
INIT_EDGE_YDELTA = $02E1         ; Temp yDelta for sub-pixel
INIT_EDGE_XCUR = $02E2           ; Temp xCurrent result
SAVED_BASE_STEP = $02E3          ; Saved baseStep from division result
SAVED_REM_LO = $02E4             ; Saved remLo from division result
SAVED_REM_HI = $02E5             ; Saved remHi from division result
SAVED_DY_LO = $02E6              ; Saved dyLo from division result
SAVED_DY_HI = $02E7              ; Saved dyHi from division result

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

; DDA edge walking state (4 edges × 11 bytes = 44 bytes)
; 16-bit Bresenham: baseStep + 16-bit remainder, 16-bit error accumulator
; Edge 0 (corner0 -> corner1)
EDGE0_X_CURRENT = $026B          ; Current X position
EDGE0_ERR_LO = $026C             ; Bresenham error accumulator lo byte
EDGE0_ERR_HI = $026D             ; Bresenham error accumulator hi byte
EDGE0_BASE_STEP = $026E          ; Base X step per scanline (quotient)
EDGE0_REM_LO = $026F             ; Remainder lo byte (added to error each step)
EDGE0_REM_HI = $0270             ; Remainder hi byte
EDGE0_DY_LO = $0271              ; dy_full lo byte (error threshold)
EDGE0_DY_HI = $0272              ; dy_full hi byte (error threshold)
EDGE0_DX_SIGN = $0273            ; Sign of dX (0=positive, 1=negative)
EDGE0_Y_START = $0274            ; First scanline (min Y)
EDGE0_Y_END = $0275              ; Last scanline (max Y)

; Edge 1 (corner1 -> corner2)
EDGE1_X_CURRENT = $0276
EDGE1_ERR_LO = $0277
EDGE1_ERR_HI = $0278
EDGE1_BASE_STEP = $0279
EDGE1_REM_LO = $027A
EDGE1_REM_HI = $027B
EDGE1_DY_LO = $027C
EDGE1_DY_HI = $027D
EDGE1_DX_SIGN = $027E
EDGE1_Y_START = $027F
EDGE1_Y_END = $0280

; Edge 2 (corner2 -> corner3)
EDGE2_X_CURRENT = $0281
EDGE2_ERR_LO = $0282
EDGE2_ERR_HI = $0283
EDGE2_BASE_STEP = $0284
EDGE2_REM_LO = $0285
EDGE2_REM_HI = $0286
EDGE2_DY_LO = $0287
EDGE2_DY_HI = $0288
EDGE2_DX_SIGN = $0289
EDGE2_Y_START = $028A
EDGE2_Y_END = $028B

; Edge 3 (corner3 -> corner0)
EDGE3_X_CURRENT = $028C
EDGE3_ERR_LO = $028D
EDGE3_ERR_HI = $028E
EDGE3_BASE_STEP = $028F
EDGE3_REM_LO = $0290
EDGE3_REM_HI = $0291
EDGE3_DY_LO = $0292
EDGE3_DY_HI = $0293
EDGE3_DX_SIGN = $0294
EDGE3_Y_START = $0295
EDGE3_Y_END = $0296

; Bufferless supersampling - stores span state for 2 supersample rows
SPAN_ROW0_X1 = $0297          ; Row 0 span start X (0-47)
SPAN_ROW0_X2 = $0298          ; Row 0 span end X (0-47)
SPAN_ROW1_X1 = $0299          ; Row 1 span start X (0-47)
SPAN_ROW1_X2 = $029A          ; Row 1 span end X (0-47)
SPAN_ROW0_VALID = $029B       ; Row 0 has valid span (0/1)
SPAN_ROW1_VALID = $029C       ; Row 1 has valid span (0/1)

; Downsampling state
DOWNSAMPLE_Y = $029D          ; Current output Y being processed
DOWNSAMPLE_X = $029E          ; Current output X being processed
SUBPIXEL_COUNT = $029F        ; Count of set subpixels (0-4)

; Wrapping state
SPAN_ROW0_X2_WRAP = $02A0     ; Row 0 wrapped X2 (x2-48)
SPAN_ROW1_X2_WRAP = $02A1     ; Row 1 wrapped X2 (x2-48)
SPAN_TEMP = $02A2             ; Temporary storage for inverted mask

; Edge division results for parallel edge optimization (2 pairs × 5 bytes = 10 bytes)
; Stores baseStep, remLo, remHi, dyLo, dyHi for shared division computation
DIV_RESULT_01_BASE = $02A3    ; Edges 0→1 and 2→3 base step
DIV_RESULT_01_REM_LO = $02A4  ; Remainder lo
DIV_RESULT_01_REM_HI = $02A5  ; Remainder hi
DIV_RESULT_01_DY_LO = $02A6   ; dy_full lo
DIV_RESULT_01_DY_HI = $02A7   ; dy_full hi

DIV_RESULT_12_BASE = $02A8    ; Edges 1→2 and 3→0 base step
DIV_RESULT_12_REM_LO = $02A9  ; Remainder lo
DIV_RESULT_12_REM_HI = $02AA  ; Remainder hi
DIV_RESULT_12_DY_LO = $02AB   ; dy_full lo
DIV_RESULT_12_DY_HI = $02AC   ; dy_full hi

; Bounds optimization for pixel loop
DOWNSAMPLE_X_MAX = $02AD      ; Output X maximum bound (outXMax)
