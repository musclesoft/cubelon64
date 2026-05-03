#!/bin/bash
# Trace frame 162 execution focusing on music player and register writes

echo "Building emulator with detailed tracing..."

# Add tracing code to emulator
cat > /tmp/trace_patch.txt <<'EOF'
Add to z80.Step() at start:

if io.CurrentFrame == 162 && cpu.PC >= 0x7EC5 && cpu.PC <= 0x7FA0 {
    fmt.Printf("[162] PC=%04X: ", cpu.PC)
    // Show instruction
}

Add before OUT instructions:

if io.CurrentFrame == 162 && (port == 0xFFFD || port == 0xBFFD) {
    fmt.Printf("[162] OUT ($%04X),A  A=%02X BC=%04X HL=%04X\n",
        port, cpu.A, cpu.BC(), cpu.HL())
}
EOF

cat /tmp/trace_patch.txt

echo ""
echo "To implement:"
echo "1. Add PC tracing in z80/cpu.go Step() method"
echo "2. Add port output tracing in spectrum/io.go"
echo "3. Run: go run emulator/run_cubelon.go --max-frames=200"
echo ""
echo "Or use existing --trace-code feature:"
echo "go run emulator/run_cubelon.go --trace-code=\"162:7EC5-7FA0\""
