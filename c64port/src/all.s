; Combined source file for single-pass assembler
; This includes all modules in the correct order

; Include all source files (they will export their symbols)
.include "sincos_tables.s"
.include "charset.s"
.include "math_helpers.s"
.include "rotation.s"
.include "render.s"
.include "animate.s"
.include "main.s"
