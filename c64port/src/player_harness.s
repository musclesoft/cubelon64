; player_harness.s — minimal standalone harness for 6502 player emulation.
;
; Assembled by tools/optimize_volume.go at runtime; the resulting PRG is loaded
; into a flat 64K memory image and driven by the m6502 CPU emulator.
;
; Layout:
;   LOADADDR header → $0801
;   music_player_init, music_player_sid_update, music_player_step, CIA handlers, music data, BSS at $0801+
;
; The harness provides no IRQ/NMI vectors — the host drives CIA firing directly
; by calling music_player_write_ctrl_ch0 / music_player_write_ctrl_ch1 at the right cycle counts.

.include "include/shared.inc"
.include "music_data.inc"
CIA1_PRB = $DC01
.include "player.s"

