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

.macro assert_flag flag, value
    ; The next instruction must run with status flag C, Z, I, D, V or N at 0 or 1 - parsed by custom assembler, ignored by ca65
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
SEQ_SCENE_STATE = $021A  ; State of the running scene to show: 1-20, or 19-0 backwards
SEQ_SCENE_ENTRY = $021D  ; The entry of the sequencer's scene table for this cycle: the event of the scene's states (EVENT_*) and SCENE_CHECKER, 0 when no scene runs
BUILD_SLOT = $0248       ; Slot of PRECOMPUTED_FRAMES the rasterizer renders into (and the frame whose corners it computes)
FRAME_TICK = $021E       ; Frames passed that the main loop has not stepped yet: counted up by the interrupt
                         ; of the last sprite band, down by the main loop
MUSIC_ON = $021F         ; Nonzero once the music runs; the demo restarts without touching it

EVENT_NONE = 0
EVENT_REVEAL = 1         ; Cover the grid and start revealing it
EVENT_MUSIC = 2          ; Start the music if it is not running; its first step is this frame's
EVENT_MORPH = 3          ; Show the morph state SEQ_SCENE_STATE
EVENT_CHECKER = 4        ; Turn rows of the grid into a checkerboard of mirrored tiles
EVENT_SPLIT = 5          ; Show the split state SEQ_SCENE_STATE
EVENT_SPIN = 6           ; Show the spin state SEQ_SCENE_STATE
EVENT_RESTART = 7        ; Start the demo over
EVENT_SPIN_MIRRORED = 8  ; Show the spin state SEQ_SCENE_STATE with every tile mirrored
EVENT_KEEP_SQUARES = 9   ; Keep the tile of the marching cubes' last frame for the morph's state 0
EVENT_UNCHECKER = 10     ; Turn rows of the checkerboard back into bands of one kind of tile
MOTION_ENDED_INDEX = 3   ; SEQ_EVENT_INDEX once the marching cubes have stopped
MORPH_STATES = 20        ; The morph runs through the states 1-20; backwards, 19-0, where 0 is the cubes' last frame
SPLIT_STATES = 20        ; The split runs through the states 1-20
SPIN_STATES = 20         ; The spin runs through the states 1-20
CHECKER_STATES = 4       ; The checkerboard is made in the states 1-4 (undone in the same number of states)
