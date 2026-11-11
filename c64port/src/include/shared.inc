; shared.inc - Variables shared between modules
; ABSOLUTE ADDRESSING ONLY - No zero page
; Base address: $0200

.include "include/layout.inc"

; ═══════════════════════════════════════════════════════════
; ASSERTION MACROS - Empty for ca65, parsed by custom assembler
; ═══════════════════════════════════════════════════════════
.macro assert_if cond, val, symbol, file, length
    ; Runtime assertion - parsed by custom assembler, ignored by ca65
.endmacro

.macro assert symbol, file, length
    ; Runtime assertion - parsed by custom assembler, ignored by ca65
.endmacro

; ═══════════════════════════════════════════════════════════
; SHARED VARIABLES - Accessible to all modules
; ═══════════════════════════════════════════════════════════

; Rotated square corner coordinates (Fixed 8.8 format)
; Written by: rotation.s, Read by: render.s
CORNER0_X = $0200        ; Corner 0 X coordinate (16-bit little-endian)
CORNER0_Y = $0202        ; Corner 0 Y coordinate (16-bit)
CORNER1_X = $0204        ; Corner 1 X coordinate (16-bit)
CORNER1_Y = $0206        ; Corner 1 Y coordinate (16-bit)
CORNER2_X = $0208        ; Corner 2 X coordinate (16-bit)
CORNER2_Y = $020A        ; Corner 2 Y coordinate (16-bit)
CORNER3_X = $020C        ; Corner 3 X coordinate (16-bit)
CORNER3_Y = $020E        ; Corner 3 Y coordinate (16-bit)

; Animation state
FRAME = $0210            ; Current animation frame (0-40 with 4-phase optimization)
RENDER_PHASE = $0211     ; Render phase: 0=compute, 1=display (for 2-frame rendering)

; Sequencer state
SEQ_STATE = $0212        ; Sequencer state counter (0-39 continuously)
SEQ_FRAME_LO = $0213     ; 16-bit frame counter (low byte)
SEQ_FRAME_HI = $0214     ; 16-bit frame counter (high byte)
SEQ_MODE = $0215         ; Current mode: 0=motion, 1=freeze
SEQ_PHASE = $0216        ; Phase counter (increments every 40 frames when state wraps)
SEQ_EVENT = $0218        ; Timeline event due this frame (EVENT_*)
SEQ_EVENT_INDEX = $0219  ; Next entry of the sequencer's event table
SEQ_SCENE_STATE = $021A  ; State (1-N) of the running scene to show, 0 before the first scene
SEQ_SCENE_INDEX = $021B  ; Next entry of the sequencer's scene table
SEQ_SCENE_LEFT = $021C   ; States of the running scene still to publish
SEQ_SCENE_KIND = $021D   ; Event of the running scene's states (EVENT_*)

EVENT_NONE = 0
EVENT_REVEAL = 1         ; Cover the grid and start revealing it
EVENT_MUSIC = 2          ; Initialize the music; its first step is this frame's
EVENT_MORPH = 3          ; Show the morph state SEQ_SCENE_STATE
EVENT_CHECKER = 4        ; Turn rows of the grid into a checkerboard of mirrored tiles
EVENT_SPLIT = 5          ; Show the split state SEQ_SCENE_STATE
EVENT_SPIN = 6           ; Show the spin state SEQ_SCENE_STATE
MUSIC_STARTED_INDEX = 2  ; SEQ_EVENT_INDEX once EVENT_MUSIC has been published
MOTION_ENDED_INDEX = 3   ; SEQ_EVENT_INDEX once the marching cubes have stopped
MORPH_STATES = 20        ; The morph runs through the states 1-20
SPLIT_STATES = 20        ; The split runs through the states 1-20
SPIN_STATES = 20         ; The spin runs through the states 1-20
CHECKER_STATES = 4       ; The checkerboard is made in the states 1-4
