; player_harness.s — minimal standalone harness for 6502 player emulation.
;
; Assembled by tools/optimize_volume.go at runtime; the resulting PRG is loaded
; into a flat 64K memory image and driven by the m6502 CPU emulator.
;
; Layout:
;   LOADADDR header → $0801
;   music_player_init, music_player_tick, CIA handlers, music data, BSS at $0801+
;
; The harness provides no IRQ/NMI vectors — the host drives CIA firing directly
; by calling player_cia1a_fire / player_cia1b_fire at the right cycle counts.

.include "include/shared.inc"
.include "music_data.inc"
CIA1_PRB = $DC01
.include "player.s"

