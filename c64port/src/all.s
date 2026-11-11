; Combined source file for single-pass assembler
; This includes all modules in the correct order

; Include all source files (they will export their symbols)
; Order matches ca65 link order: main must be first for BASIC stub
.include "main.s"
.include "rotation.s"
.include "charset.s"
.include "render.s"
.include "math_helpers.s"
.include "sincos_tables.s"
.include "bit_reverse_table.s"
.include "vic_init.s"
.include "cia_init.s"
.include "animate.s"
.include "sequencer.s"
.include "square_sprite_mux.s"
;.include "sprite_mux.s" ; simple 24x21px sprites
.include "reveal.s"
.include "tiles.s"
.include "morph.s"
.include "morph_table.inc"
.include "split.s"
.include "split_table.inc"
.include "spin.s"
.include "spin_data.inc"
.include "music_data.inc"
CIA1_PRB = $DC01
.include "player.s"
.include "irq.s"
.include "job.s"
