# CUBELON Part 2 - Animation Viewer

## Full Animation Visualization

Open `animation.html` in your browser to see the complete 52-frame animation.

**Features:**
- **Slider**: Step through all 52 frames (frames 319-370)
- **Play button**: Auto-play the animation at 25 fps
- **Keyboard controls**: Arrow keys to step, Space to play/pause
- **Frame info**: Shows current frame number

**Data:**
- `animation_frames.json` - 52 frames of 24×12 pixel data extracted from ZX Spectrum screen memory
- Each frame captured at end of frame after drawing completes
- Pixel-perfect reproduction of the actual demo

**Animation cycle:**
- Frames 319-370 show the complete "square emergence" effect
- One of 64 squares rotating/emerging from center
- Part of the larger 8×8 grid of rotating cubes

The visualization shows the actual screen output as rendered by the ZX Spectrum,
extracted directly from screen memory (Bank 5, Y=6-17, X=4-6 bytes).
