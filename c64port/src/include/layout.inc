; layout.inc - Where the graphics live
;
; The VIC reads bank 1 ($4000-$7FFF): four screens, the charset and the sprite data.
; Screens A and C show chars, B and D only hold sprite pointers (see square_sprite_mux.s).

SCREEN = $4000
SCREEN_A = SCREEN
SCREEN_B = $4400
SCREEN_C = $4800
SCREEN_D = $4C00
COLOR = $D800                       ; Color RAM, not in the VIC bank
CHARSET = $6000
SPRITE = $6800                      ; The normal sprite, its partner at +$40
SPRITE_REVERSE = $6880              ; The mirrored sprite, its partner at +$40
SPRITE_POINTER_NORMAL = $A0         ; SPRITE / 64
SPRITE_POINTER_MIRROR = $A2         ; SPRITE_REVERSE / 64
