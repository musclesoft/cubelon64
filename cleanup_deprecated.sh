#!/bin/bash
# Clean up deprecated files after methodology change
# See docs/METHODOLOGY_SCREEN_EXTRACTION.md for rationale

echo "=== Cleaning up deprecated files ==="
echo ""

# Remove compiled binaries (will be ignored by .gitignore)
echo "Removing compiled binaries..."
rm -f emulator/cubelon-emu
rm -f emulator/run_cubelon
rm -f tools/dump_pattern_table
rm -f tools/trace_pattern_reads

# Remove deprecated JSON exports (screen-based, now invalid)
echo "Removing deprecated JSON exports..."
rm -f emulator/animation_frames_*.json
rm -f emulator/part3_tiles_*.json
rm -f tools/part3_tiles_*.json

# Remove execution traces
echo "Removing execution traces..."
rm -f emulator/execution_stats.txt
rm -f tools/code_trace.log
rm -f tools/screen_trace.log
rm -f tools/part3_complete_trace.json

# Remove deprecated PNG visualizations (generated from screen extraction)
echo "Removing deprecated PNG visualizations..."
rm -f tools/animation_sprite_sheet_source.png
rm -f tools/decode_column_major.png
rm -f tools/decode_linear.png
rm -f tools/frame900_*.png
rm -f tools/frame920_*.png
rm -f tools/frame921_*.png
rm -f tools/screen920_*.png
rm -f tools/part2_all_unique_patterns.png
rm -f tools/part2_chunk_reuse.png
rm -f tools/part2_complete_all_patterns.png
rm -f tools/part3_all_12_patterns.png
rm -f tools/part3_all_patterns_CORRECT.png
rm -f tools/part3_all_unique_patterns_COMPLETE.png
rm -f tools/part3_chunk_reuse.png
rm -f tools/part3_complete_all_patterns.png
rm -f tools/part3_pattern_*.png
rm -f tools/part3_tile_frame*.png
rm -f tools/part3_tiles_*_sheet.png
rm -f tools/part3_tiles_NEW_BC_CAPTURE.png
rm -f tools/part3_tiles_*_CAPTURED.png
rm -f tools/part4_chunk_reuse.png
rm -f tools/part4_complete_all_patterns.png
rm -f tools/part5_chunk_reuse.png
rm -f tools/part5_complete_all_patterns.png

echo ""
echo "=== Cleanup complete ==="
echo ""
echo "Files KEPT (still valid):"
echo "  - docs/*.md (all documentation)"
echo "  - emulator/full_disassembly.asm (disassembly with navigation constants)"
echo "  - tools/part3_write_sequence.json (write permutation table)"
echo "  - tools/*.py (deprecated but kept for reference)"
echo "  - tools/README.md (if exists)"
echo ""
echo "Files REMOVED:"
echo "  - All JSON exports (deprecated)"
echo "  - All PNG visualizations (screen-based, deprecated)"
echo "  - Compiled binaries (ignored by .gitignore)"
echo "  - Execution traces (temporary)"
echo ""
echo "See docs/METHODOLOGY_SCREEN_EXTRACTION.md for details"
